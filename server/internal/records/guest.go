package records

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kintion/server/internal/audit"
	"kintion/server/internal/db/sqlc"
)

// Гость-редактор (0.5, F-04, F-148, F-149): своё созданное гость правит
// через техническую группу «Созданные <ник>»; право правки на неё — право
// создавать записи.

// seenLink — привязка, если её цель видна смотрящему (F-05): скрытая цель — без привязки.
func (t *Tx) seenLink(id *uuid.UUID) *uuid.UUID {
	if id == nil || !t.Guest() {
		return id
	}
	rows, err := t.visibleRows([]uuid.UUID{*id})
	if err != nil || len(rows) == 0 {
		return nil
	}
	return id
}

// GuestAudit — запись журнала из действий гостя глазами гостя (F-05, F-08):
// ссылки на невидимые ему записи, которые в снимок могли попасть от чужих
// правок (привязки дат записи, привязки и ссылка значения), убираются;
// привязка значения к невидимой записи не показывается вовсе (show = false).
func (t *Tx) GuestAudit(object string, before, after []byte) (b, a []byte, show bool) {
	if !t.Guest() {
		return before, after, true
	}
	switch object {
	case "entity":
		return t.scrub(before, t.scrubEntity), t.scrub(after, t.scrubEntity), true
	case "field_entry":
		return t.scrub(before, t.scrubValue), t.scrub(after, t.scrubValue), true
	case "entry_link":
		for _, raw := range [][]byte{before, after} {
			var l struct {
				Target *uuid.UUID `json:"target_id"`
			}
			if len(raw) > 0 && json.Unmarshal(raw, &l) == nil && l.Target != nil && t.seenLink(l.Target) == nil {
				return nil, nil, false
			}
		}
	}
	return before, after, true
}

// scrub применяет fix к снимку-объекту; не объект — как есть.
func (t *Tx) scrub(raw []byte, fix func(map[string]json.RawMessage) bool) []byte {
	if len(raw) == 0 || string(raw) == "null" {
		return raw
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil || !fix(m) {
		return raw
	}
	out, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return out
}

// hiddenID — значение — id записи, невидимой смотрящему.
func (t *Tx) hiddenID(raw json.RawMessage) bool {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return false
	}
	id, err := uuid.Parse(s)
	return err == nil && t.seenLink(&id) == nil
}

func (t *Tx) scrubEntity(m map[string]json.RawMessage) bool {
	changed := false
	for _, k := range []string{"start_link", "end_link"} {
		if t.hiddenID(m[k]) {
			m[k] = json.RawMessage("null")
			changed = true
		}
	}
	return changed
}

func (t *Tx) scrubValue(m map[string]json.RawMessage) bool {
	changed := false
	if t.hiddenID(m["value"]) {
		m["value"] = json.RawMessage("null")
		changed = true
	}
	var links []LinkDTO
	if json.Unmarshal(m["links"], &links) == nil && len(links) > 0 {
		kept := links[:0]
		for _, l := range links {
			if t.seenLink(&l.Target) != nil {
				kept = append(kept, l)
			}
		}
		if len(kept) != len(links) {
			if b, err := json.Marshal(kept); err == nil {
				m["links"] = b
				changed = true
			}
		}
	}
	return changed
}

// adopt — новая запись гостя видна и доступна ему на правку в этой же
// транзакции (метка технической группы ставится позже в пакете).
func (t *Tx) adopt(id uuid.UUID) {
	if t.Guest() {
		t.scope[id] = true
		t.editScope[id] = true
	}
}

// ownGroupID — техническая группа гостя в дашборде (F-149): создаётся при
// принятии приглашения (EnsureGuestGroup); без неё гость не создаёт записей.
func (t *Tx) ownGroupID() (uuid.UUID, error) {
	if t.ownGroup != nil {
		return *t.ownGroup, nil
	}
	g, err := t.q.LockGuestGroup(t.ctx, sqlc.LockGuestGroupParams{DashboardID: t.Dashboard, CreatedFor: &t.guest})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrForbidden
	} else if err != nil {
		return uuid.Nil, err
	}
	t.ownGroup = &g.ID
	return g.ID, nil
}

// EnsureGuestGroup — техническая группа гостя guest (F-149): прежняя (гость
// уходил и вернулся) или новая с названием name. Право правки на неё — право
// создавать записи; если права у гостя нет, оно выдаётся правкой, когда у
// гостя есть право правки хоть на одну группу, иначе — просмотром. Выдаётся
// при принятии приглашения (accept) или вместе с новой группой; дальше этим
// правом распоряжается владелец — отозванное не возвращается.
func (t *Tx) EnsureGuestGroup(guest uuid.UUID, name string, accept bool) error {
	g, err := t.q.LockGuestGroup(t.ctx, sqlc.LockGuestGroupParams{DashboardID: t.Dashboard, CreatedFor: &guest})
	if err == nil && !accept {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		if g, err = t.CreateGroup(NewGroup{Name: name}); err != nil {
			return err
		}
		if err := t.q.SetGroupCreatedFor(t.ctx, sqlc.SetGroupCreatedForParams{ID: g.ID, CreatedFor: &guest}); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if _, err := t.q.GetGrant(t.ctx, sqlc.GetGrantParams{DashboardID: t.Dashboard, GroupID: g.ID, AccountID: guest}); err == nil {
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	grants, err := t.q.ListMemberGrants(t.ctx, sqlc.ListMemberGrantsParams{DashboardID: t.Dashboard, AccountID: guest})
	if err != nil {
		return err
	}
	edit := slices.ContainsFunc(grants, func(gr sqlc.GrpGrant) bool { return gr.CanEdit })
	gr, err := t.q.UpsertGrant(t.ctx, sqlc.UpsertGrantParams{DashboardID: t.Dashboard, GroupID: g.ID, AccountID: guest, CanEdit: edit})
	if err != nil {
		return err
	}
	acc, err := t.q.GetAccount(t.ctx, guest)
	if err != nil {
		return err
	}
	return t.audit(audit.Entry{Object: "grp_grant", ObjectID: &g.ID, Op: audit.OpCreate,
		After: grantSnap{AccountID: guest, Nick: acc.Nick, GroupID: g.ID, CanEdit: gr.CanEdit}})
}

// RenameGuestGroup — смена ника гостя (F-149): в названии его технической
// группы старый ник заменяется новым, даже если владелец её переименовал.
func (t *Tx) RenameGuestGroup(g sqlc.Grp, oldNick, newNick string) error {
	name := replaceNick(g.Name, oldNick, newNick)
	// длиннее предела названия — не переименовывается: смену ника это не держит
	if name == g.Name || utf8.RuneCountInString(name) > 200 {
		return nil
	}
	_, err := t.PatchGroup(g, PatchGroup{Name: Opt[string]{Set: true, V: &name}})
	return err
}

// replaceNick заменяет в s ник old на new — только целым словом: рядом с
// вхождением нет символов ника (буква, цифра, «.», «_», «-»).
func replaceNick(s, old, new string) string {
	if old == "" {
		return s
	}
	nickRune := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._-", r) }
	// границы слов — по исходной строке целиком
	var b strings.Builder
	last := 0
	for i := 0; i <= len(s)-len(old); {
		if !strings.HasPrefix(s[i:], old) {
			_, w := utf8.DecodeRuneInString(s[i:])
			i += w
			continue
		}
		before, _ := utf8.DecodeLastRuneInString(s[:i])
		after, _ := utf8.DecodeRuneInString(s[i+len(old):])
		if (i == 0 || !nickRune(before)) && (i+len(old) == len(s) || !nickRune(after)) {
			b.WriteString(s[last:i])
			b.WriteString(new)
			i += len(old)
			last = i
			continue
		}
		_, w := utf8.DecodeRuneInString(s[i:])
		i += w
	}
	b.WriteString(s[last:])
	return b.String()
}

// labelOwn ставит записи гостя метку его технической группы.
func (t *Tx) labelOwn(entity uuid.UUID) error {
	g, err := t.ownGroupID()
	if err != nil {
		return err
	}
	cur, err := t.q.ListGroupsByEntities(t.ctx, []uuid.UUID{entity})
	if err != nil {
		return err
	}
	for _, c := range cur {
		if c.GroupID == g {
			return nil
		}
	}
	if err := t.q.InsertEntityGroup(t.ctx, sqlc.InsertEntityGroupParams{DashboardID: t.Dashboard, EntityID: entity, GroupID: g, CreatedBy: t.actor()}); err != nil {
		return err
	}
	return t.auditLabel(audit.OpCreate, entity, g)
}

// guestLabels — метки, которые гость хочет видеть на записи (want), с учётом
// прав (F-148): ставить и снимать он может только метки групп с правкой;
// остальные текущие метки (невидимые, только для чтения, своя техническая)
// сохраняются как есть.
func (t *Tx) guestLabels(entity uuid.UUID, want []uuid.UUID, field string) ([]uuid.UUID, error) {
	cur, err := t.q.ListGroupsByEntities(t.ctx, []uuid.UUID{entity})
	if err != nil {
		return nil, err
	}
	have := map[uuid.UUID]bool{}
	for _, c := range cur {
		have[c.GroupID] = true
	}
	for _, g := range want {
		// уже стоящая метка без права правки — не изменение: она и так сохранится
		if t.GroupEditable(g) || (have[g] && t.GroupVisible(g)) {
			continue
		}
		if t.GroupVisible(g) {
			return nil, invalid(field, "нет права ставить метку этой группы")
		}
		return nil, invalid(field, "нет группы %s", g)
	}
	out := append([]uuid.UUID{}, want...)
	for _, c := range cur {
		if !t.GroupEditable(c.GroupID) {
			out = append(out, c.GroupID)
		}
	}
	return out, nil
}
