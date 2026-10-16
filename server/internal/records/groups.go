package records

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kintion/server/internal/audit"
	"kintion/server/internal/db/sqlc"
)

// Группы (F-02): метки записей дашборда; запись — в нескольких группах.
// Вложенность — многие ко многим (F-105): группа входит в несколько групп,
// права наследуются вниз по всем путям, циклов нет. В 0.1 без прав (F-04 —
// часть владельца).

// GroupDTO — группа в ответах; Count — число видимых записей с её меткой,
// Parents («Входит в») и Children («Включает в себя») — видимые группы (F-07),
// в порядке списка.
type GroupDTO struct {
	ID       uuid.UUID   `json:"id"`
	Name     string      `json:"name"`
	Count    int64       `json:"count"`
	Parents  []uuid.UUID `json:"parents"`
	Children []uuid.UUID `json:"children"`
	// AutoSurname и Surnames — автодобавление записей по фамилии (F-122)
	AutoSurname bool     `json:"auto_surname"`
	Surnames    []string `json:"surnames"`
}

// NewGroup — POST /api/dashboards/{d}/groups (по умолчанию — в конец списка).
type NewGroup struct {
	Name        string      `json:"name"`
	Parents     []uuid.UUID `json:"parents"`
	Children    []uuid.UUID `json:"children"`
	AutoSurname bool        `json:"auto_surname"`
	Surnames    []string    `json:"surnames"`
	Position
}

// PatchGroup — переименовать, переставить, сменить вложенность (списки
// «Входит в» и «Включает в себя» — целиком; нет ключа — без изменений).
type PatchGroup struct {
	Name        Opt[string]  `json:"name"`
	Parents     *[]uuid.UUID `json:"parents"`
	Children    *[]uuid.UUID `json:"children"`
	AutoSurname *bool        `json:"auto_surname"`
	Surnames    *[]string    `json:"surnames"`
	Position
}

// SetGroups — PUT /api/records/{id}/groups: метки записи целиком.
type SetGroups struct {
	IfRev  *int64      `json:"if_rev"`
	Groups []uuid.UUID `json:"groups"`
}

type groupSnap struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	SortKey     string    `json:"sort_key"`
	AutoSurname bool      `json:"auto_surname"`
	Surnames    []string  `json:"surnames"`
}

func snapGroup(g sqlc.Grp) groupSnap {
	return groupSnap{ID: g.ID, Name: g.Name, SortKey: g.SortKey, AutoSurname: g.AutoSurname, Surnames: g.Surnames}
}

// groupLinkSnap — запись журнала о вложенности: child входит в parent.
type groupLinkSnap struct {
	ParentID uuid.UUID `json:"parent_id"`
	ChildID  uuid.UUID `json:"child_id"`
}

// groupLink — связь «child входит в parent».
type groupLink struct{ parent, child uuid.UUID }

// Ключи входа по тегам (строки в коде не повторяются).
var (
	keyParents  = tagOf(NewGroup{}, "Parents")
	keyChildren = tagOf(NewGroup{}, "Children")
	keySurnames = tagOf(NewGroup{}, "Surnames")
)

func (t *Tx) auditGroup(op string, before, after *sqlc.Grp) error {
	e := audit.Entry{Object: "grp", Op: op}
	if before != nil {
		e.ObjectID, e.Before = &before.ID, snapGroup(*before)
	}
	if after != nil {
		e.ObjectID, e.After = &after.ID, snapGroup(*after)
	}
	return t.audit(e)
}

func (t *Tx) auditGroupLink(op string, l groupLink) error {
	s := groupLinkSnap{ParentID: l.parent, ChildID: l.child}
	e := audit.Entry{Object: "grp_link", ObjectID: &l.child, Op: op}
	if op == audit.OpDelete {
		e.Before = s
	} else {
		e.After = s
	}
	return t.audit(e)
}

func (t *Tx) insertGroupLink(l groupLink) error {
	if err := t.q.InsertGroupLink(t.ctx, sqlc.InsertGroupLinkParams{DashboardID: t.Dashboard, ParentID: l.parent, ChildID: l.child}); err != nil {
		return err
	}
	return t.auditGroupLink(audit.OpCreate, l)
}

func (t *Tx) deleteGroupLink(l groupLink) error {
	if err := t.q.DeleteGroupLink(t.ctx, sqlc.DeleteGroupLinkParams{ParentID: l.parent, ChildID: l.child}); err != nil {
		return err
	}
	return t.auditGroupLink(audit.OpDelete, l)
}

// groupLinks — вложенность групп дашборда под блокировкой: правки идут по
// одной, проверка циклов видит согласованный граф.
func (t *Tx) groupLinks() ([]groupLink, error) {
	if err := t.q.LockGroupLinks(t.ctx, t.Dashboard.String()); err != nil {
		return nil, err
	}
	rows, err := t.q.ListGroupLinks(t.ctx, t.Dashboard)
	if err != nil {
		return nil, err
	}
	out := make([]groupLink, 0, len(rows))
	for _, r := range rows {
		out = append(out, groupLink{r.ParentID, r.ChildID})
	}
	return out, nil
}

func (t *Tx) auditLabel(op string, entity, group uuid.UUID) error {
	s := labelSnap{EntityID: entity, GroupID: group}
	e := audit.Entry{Object: "entity_group", ObjectID: &group, Entity: &entity, Op: op}
	if op == audit.OpDelete {
		e.Before = s
	} else {
		e.After = s
	}
	return t.audit(e)
}

func groupName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > 200 {
		return "", invalid(keyName, "название: от 1 до 200 символов")
	}
	return s, nil
}

// Group — группа дашборда по id.
func (t *Tx) Group(id uuid.UUID) (sqlc.Grp, error) {
	g, err := t.q.GetGroup(t.ctx, sqlc.GetGroupParams{DashboardID: t.Dashboard, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return g, ErrNotFound
	}
	return g, err
}

// Groups — все группы дашборда плоским списком (F-105), порядок — ручной.
func (t *Tx) Groups() ([]GroupDTO, error) {
	rows, err := t.q.ListGroups(t.ctx, t.Dashboard)
	if err != nil {
		return nil, err
	}
	counts, err := t.q.CountGroupLabels(t.ctx, t.Dashboard)
	if err != nil {
		return nil, err
	}
	links, err := t.q.ListGroupLinks(t.ctx, t.Dashboard)
	if err != nil {
		return nil, err
	}
	n := map[uuid.UUID]int64{}
	for _, c := range counts {
		n[c.GroupID] = c.N
	}
	order := make(map[uuid.UUID]int, len(rows))
	for i, g := range rows {
		order[g.ID] = i
	}
	parents := map[uuid.UUID][]uuid.UUID{}
	children := map[uuid.UUID][]uuid.UUID{}
	for _, l := range links {
		parents[l.ChildID] = append(parents[l.ChildID], l.ParentID)
		children[l.ParentID] = append(children[l.ParentID], l.ChildID)
	}
	visible := func(ids []uuid.UUID) []uuid.UUID {
		out := make([]uuid.UUID, 0, len(ids))
		for _, id := range ids {
			if _, ok := order[id]; ok { // только видимые группы (F-07)
				out = append(out, id)
			}
		}
		slices.SortFunc(out, func(a, b uuid.UUID) int { return order[a] - order[b] })
		return out
	}
	out := make([]GroupDTO, 0, len(rows))
	for _, g := range rows {
		out = append(out, GroupDTO{ID: g.ID, Name: g.Name, Count: n[g.ID],
			Parents: visible(parents[g.ID]), Children: visible(children[g.ID]),
			AutoSurname: g.AutoSurname, Surnames: g.Surnames})
	}
	return out, nil
}

// GroupOut — группа для ответа.
func (t *Tx) GroupOut(g sqlc.Grp) (GroupDTO, error) {
	all, err := t.Groups()
	if err != nil {
		return GroupDTO{}, err
	}
	for _, d := range all {
		if d.ID == g.ID {
			return d, nil
		}
	}
	return GroupDTO{}, ErrNotFound
}

// groupKey — ключ порядка в списке групп (без самой группы self).
func (t *Tx) groupKey(self uuid.UUID, pos Position) (string, error) {
	rows, err := t.q.ListGroups(t.ctx, t.Dashboard)
	if err != nil {
		return "", err
	}
	var keys []string
	var ids []uuid.UUID
	for _, g := range rows {
		if g.ID != self {
			keys, ids = append(keys, g.SortKey), append(ids, g.ID)
		}
	}
	if !pos.set() {
		pos.Position = posBottom
	}
	return between(keys, ids, pos, "position")
}

// setGroupLinks приводит «Входит в» (parents) и «Включает в себя» (children)
// группы g к заданным спискам (nil — без изменений) с журналом каждой связи;
// группа не может оказаться внутри самой себя (F-105).
func (t *Tx) setGroupLinks(g uuid.UUID, parents, children *[]uuid.UUID) error {
	if parents == nil && children == nil {
		return nil
	}
	links, err := t.groupLinks()
	if err != nil {
		return err
	}
	rows, err := t.q.ListGroups(t.ctx, t.Dashboard)
	if err != nil {
		return err
	}
	exists := map[uuid.UUID]bool{}
	for _, r := range rows {
		exists[r.ID] = true
	}
	have := map[groupLink]bool{}
	for _, l := range links {
		have[l] = true
	}
	// одна сторона: mk — связь с другой группой, mine — связь этой стороны у g
	side := func(ids *[]uuid.UUID, field string, mk func(other uuid.UUID) groupLink, mine func(l groupLink) bool) error {
		if ids == nil {
			return nil
		}
		want := map[groupLink]bool{}
		for _, id := range *ids {
			if id == g {
				return invalid(field, "группа не может входить в саму себя")
			}
			if !exists[id] {
				return invalid(field, "нет группы %s", id)
			}
			l := mk(id)
			if want[l] {
				continue
			}
			want[l] = true
			if have[l] {
				continue
			}
			if err := t.insertGroupLink(l); err != nil {
				return err
			}
			have[l] = true
		}
		for _, l := range links {
			if mine(l) && !want[l] {
				if err := t.deleteGroupLink(l); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := side(parents, keyParents,
		func(p uuid.UUID) groupLink { return groupLink{p, g} },
		func(l groupLink) bool { return l.child == g }); err != nil {
		return err
	}
	if err := side(children, keyChildren,
		func(c uuid.UUID) groupLink { return groupLink{g, c} },
		func(l groupLink) bool { return l.parent == g }); err != nil {
		return err
	}
	cycle, err := t.q.GroupInCycle(t.ctx, g)
	if err != nil {
		return err
	}
	if cycle {
		field := keyParents
		if parents == nil {
			field = keyChildren
		}
		return invalid(field, "группа не может оказаться внутри самой себя")
	}
	return nil
}

// CreateGroup создаёт группу; имена не уникальны (F-07).
func (t *Tx) CreateGroup(in NewGroup) (sqlc.Grp, error) {
	name, err := groupName(in.Name)
	if err != nil {
		return sqlc.Grp{}, err
	}
	id := newID()
	sk, err := t.groupKey(id, in.Position)
	if err != nil {
		return sqlc.Grp{}, err
	}
	surnames, err := cleanSurnames(in.Surnames)
	if err != nil {
		return sqlc.Grp{}, err
	}
	g, err := t.q.InsertGroup(t.ctx, sqlc.InsertGroupParams{ID: id, DashboardID: t.Dashboard, Name: name, SortKey: sk,
		AutoSurname: in.AutoSurname, Surnames: surnames})
	if err != nil {
		return g, err
	}
	if err := t.auditGroup(audit.OpCreate, nil, &g); err != nil {
		return g, err
	}
	var parents, children *[]uuid.UUID
	if len(in.Parents) > 0 {
		parents = &in.Parents
	}
	if len(in.Children) > 0 {
		children = &in.Children
	}
	if err := t.setGroupLinks(g.ID, parents, children); err != nil {
		return g, err
	}
	return g, t.autoGroupExisting(g)
}

// PatchGroup переименовывает, переставляет группу и меняет её вложенность.
func (t *Tx) PatchGroup(old sqlc.Grp, in PatchGroup) (sqlc.Grp, error) {
	g := old
	if in.Name.Set {
		if in.Name.V == nil {
			return g, invalid(keyName, "нужно название")
		}
		name, err := groupName(*in.Name.V)
		if err != nil {
			return g, err
		}
		g.Name = name
	}
	if in.set() {
		sk, err := t.groupKey(g.ID, in.Position)
		if err != nil {
			return g, err
		}
		g.SortKey = sk
	}
	if in.AutoSurname != nil {
		g.AutoSurname = *in.AutoSurname
	}
	if in.Surnames != nil {
		sn, err := cleanSurnames(*in.Surnames)
		if err != nil {
			return g, err
		}
		g.Surnames = sn
	}
	if err := t.setGroupLinks(g.ID, in.Parents, in.Children); err != nil {
		return g, err
	}
	if g.Name == old.Name && g.SortKey == old.SortKey && g.AutoSurname == old.AutoSurname && slices.Equal(g.Surnames, old.Surnames) {
		return g, nil
	}
	ng, err := t.q.UpdateGroup(t.ctx, sqlc.UpdateGroupParams{ID: g.ID, Name: g.Name, SortKey: g.SortKey,
		AutoSurname: g.AutoSurname, Surnames: g.Surnames})
	if err != nil {
		return ng, err
	}
	op := audit.OpUpdate
	if ng.Name == old.Name && ng.AutoSurname == old.AutoSurname && slices.Equal(ng.Surnames, old.Surnames) {
		op = audit.OpMove
	}
	if err := t.auditGroup(op, &old, &ng); err != nil {
		return ng, err
	}
	if ng.AutoSurname == old.AutoSurname && slices.Equal(ng.Surnames, old.Surnames) {
		return ng, nil // переименование и перестановка не возвращают убранных вручную
	}
	return ng, t.autoGroupExisting(ng)
}

// DeleteGroup удаляет группу (Д-29): метки снимаются с записей, её связи
// вложенности разрываются — вложенные группы к её родителям не переходят
// (F-105); записи не удаляются.
func (t *Tx) DeleteGroup(g sqlc.Grp) error {
	labels, err := t.q.ListLabelsByGroup(t.ctx, g.ID)
	if err != nil {
		return err
	}
	for _, e := range labels {
		if err := t.q.DeleteEntityGroup(t.ctx, sqlc.DeleteEntityGroupParams{EntityID: e, GroupID: g.ID}); err != nil {
			return err
		}
		if err := t.auditLabel(audit.OpDelete, e, g.ID); err != nil {
			return err
		}
		if _, err := t.bumpRev(e, RevGroups); err != nil {
			return err
		}
	}
	links, err := t.groupLinks()
	if err != nil {
		return err
	}
	for _, l := range links {
		if l.child == g.ID || l.parent == g.ID {
			if err := t.deleteGroupLink(l); err != nil {
				return err
			}
		}
	}
	if err := t.q.DeleteGroup(t.ctx, g.ID); err != nil {
		return err
	}
	return t.auditGroup(audit.OpDelete, &g, nil)
}

// SetGroups заменяет метки записи (версия groups — F-45).
func (t *Tx) SetGroups(r *Rec, in SetGroups) (*Rec, error) {
	if err := live(r); err != nil {
		return nil, err
	}
	if err := t.lockRev(r.Row.ID, RevGroups, in.IfRev, t.systemGroupsCurrent(r)); err != nil {
		return nil, err
	}
	if err := t.setLabels(r.Row.ID, in.Groups, "groups"); err != nil {
		return nil, err
	}
	return t.bumped(r, RevGroups)
}

// GroupMembers — массовые метки одной группы (F-131): добавить записям Add,
// снять с записей Remove.
type GroupMembers struct {
	Add    []uuid.UUID `json:"add"`
	Remove []uuid.UUID `json:"remove"`
}

// MembersOut — сколько меток изменилось (уже стоявшие и уже снятые не считаются).
type MembersOut struct {
	Changed int `json:"changed"`
}

// maxBulk — предел записей одной массовой операции.
const maxBulk = 1000

// SetGroupMembers ставит и снимает метку группы g у многих записей одной
// операцией: журнал и версия groups — у каждой записи, как у SetGroups.
// Версия не сверяется (if_rev нет): метка одной группы ставится поверх
// текущего состава, остальные метки записи не трогаются.
// Невидимая или удалённая запись — ErrNotFound, как у одиночной (F-07).
func (t *Tx) SetGroupMembers(g uuid.UUID, in GroupMembers) (MembersOut, error) {
	var out MembersOut
	if _, err := t.Group(g); err != nil {
		return out, err
	}
	ids := append(append([]uuid.UUID{}, in.Add...), in.Remove...)
	if len(ids) == 0 {
		return out, invalid("records", "нет записей")
	}
	if len(ids) > maxBulk {
		return out, invalid("records", "не больше %d записей", maxBulk)
	}
	adding := map[uuid.UUID]bool{}
	for _, id := range in.Add {
		adding[id] = true
	}
	for _, id := range in.Remove {
		if adding[id] {
			return out, invalid("records", "запись %s и добавляется, и убирается", id)
		}
	}
	rows, err := t.visibleRows(ids)
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		if _, ok := rows[id]; !ok {
			return out, ErrNotFound
		}
	}
	// версия groups каждой записи — под блокировку до чтения меток (в одном
	// порядке, без взаимных блокировок): параллельная правка ждёт, одиночная
	// с устаревшей версией после нас получит 409 (F-45)
	locked := slices.Clone(ids)
	slices.SortFunc(locked, func(x, y uuid.UUID) int { return strings.Compare(x.String(), y.String()) })
	for i, id := range locked {
		if i > 0 && locked[i-1] == id {
			continue
		}
		if _, err := t.q.LockFieldRev(t.ctx, sqlc.LockFieldRevParams{EntityID: id, FieldKey: RevGroups}); err != nil {
			return out, err
		}
	}
	cur, err := t.q.ListGroupsByEntities(t.ctx, ids)
	if err != nil {
		return out, err
	}
	has := map[uuid.UUID]bool{}
	for _, c := range cur {
		if c.GroupID == g {
			has[c.EntityID] = true
		}
	}
	done := map[uuid.UUID]bool{}
	for _, id := range ids {
		if done[id] || adding[id] == has[id] {
			continue
		}
		done[id] = true
		if adding[id] {
			err = t.q.InsertEntityGroup(t.ctx, sqlc.InsertEntityGroupParams{DashboardID: t.Dashboard, EntityID: id, GroupID: g, CreatedBy: t.actor()})
		} else {
			err = t.q.DeleteEntityGroup(t.ctx, sqlc.DeleteEntityGroupParams{EntityID: id, GroupID: g})
		}
		if err != nil {
			return out, err
		}
		op := audit.OpCreate
		if !adding[id] {
			op = audit.OpDelete
		}
		if err := t.auditLabel(op, id, g); err != nil {
			return out, err
		}
		if _, err := t.bumpRev(id, RevGroups); err != nil {
			return out, err
		}
		out.Changed++
	}
	return out, nil
}

func (t *Tx) systemGroupsCurrent(r *Rec) func() (any, error) {
	return func() (any, error) {
		d, err := t.Envelope(r)
		if err != nil {
			return nil, err
		}
		return systemValue{Rev: d.Revs[RevGroups], Value: d.Groups}, nil
	}
}

// actor — автор операции (nil — без автора).
func (t *Tx) actor() *uuid.UUID {
	if t.op == nil {
		return nil
	}
	return t.op.Actor
}

// RecentGroups — последние уникальные группы, назначенные аккаунтом в
// дашборде (F-81): по меткам «запись — группа», новые первыми.
func (t *Tx) RecentGroups(account uuid.UUID, limit int32) ([]uuid.UUID, error) {
	return t.q.RecentGroupsByAccount(t.ctx, sqlc.RecentGroupsByAccountParams{DashboardID: t.Dashboard, AccountID: account, Lim: limit})
}

// setLabels приводит метки записи к want с журналом каждой метки.
func (t *Tx) setLabels(entity uuid.UUID, want []uuid.UUID, field string) error {
	all, err := t.q.ListGroups(t.ctx, t.Dashboard)
	if err != nil {
		return err
	}
	for _, g := range want {
		if !slices.ContainsFunc(all, func(x sqlc.Grp) bool { return x.ID == g }) {
			return invalid(field, "нет группы %s", g)
		}
	}
	cur, err := t.q.ListGroupsByEntities(t.ctx, []uuid.UUID{entity})
	if err != nil {
		return err
	}
	have := map[uuid.UUID]bool{}
	for _, c := range cur {
		have[c.GroupID] = true
	}
	keep := map[uuid.UUID]bool{}
	for _, g := range want {
		if keep[g] {
			continue
		}
		keep[g] = true
		if have[g] {
			continue
		}
		if err := t.q.InsertEntityGroup(t.ctx, sqlc.InsertEntityGroupParams{DashboardID: t.Dashboard, EntityID: entity, GroupID: g, CreatedBy: t.actor()}); err != nil {
			return err
		}
		if err := t.auditLabel(audit.OpCreate, entity, g); err != nil {
			return err
		}
	}
	for _, c := range cur {
		if keep[c.GroupID] {
			continue
		}
		if err := t.q.DeleteEntityGroup(t.ctx, sqlc.DeleteEntityGroupParams{EntityID: entity, GroupID: c.GroupID}); err != nil {
			return err
		}
		if err := t.auditLabel(audit.OpDelete, entity, c.GroupID); err != nil {
			return err
		}
	}
	return nil
}

// ListQuery — фильтры списка записей дашборда (теги — параметры запроса).
type ListQuery struct {
	Entity  string     `json:"entity"`
	Group   *uuid.UUID `json:"group"`
	Q       string     `json:"q"`
	Deleted bool       `json:"deleted"` // корзина
	Limit   int        `json:"limit"`
	Offset  int        `json:"offset"`
}

// Ключи входа по тегам (строки ключей паков в коде не повторяются, план §3 п.7).
var (
	keyName  = tagOf(NewGroup{}, "Name")
	KeyGroup = tagOf(ListQuery{}, "Group")
)

func tagOf(v any, field string) string {
	f, _ := reflect.TypeOf(v).FieldByName(field)
	key, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	return key
}

// MaxLimit — наибольшая страница списка.
const MaxLimit = 200

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// List — страница записей и общее число найденных (только видимые, F-07).
func (t *Tx) List(lq ListQuery) ([]*Rec, int64, error) {
	// порядок задаёт пак (F-133), а он не выражается в SQL: берём все id, сортируем, режем страницу
	p := sqlc.ListRecordsParams{DashboardID: t.Dashboard, Deleted: lq.Deleted, Lim: math.MaxInt32}
	lim, off := 50, max(lq.Offset, 0)
	if lq.Limit > 0 {
		lim = min(lq.Limit, MaxLimit)
	}
	if lq.Entity != "" {
		p.Entity = &lq.Entity
	}
	if lq.Group != nil {
		if _, err := t.Group(*lq.Group); err != nil {
			return nil, 0, invalid(KeyGroup, "нет такой группы")
		}
		p.GroupID = lq.Group
	}
	if q := strings.TrimSpace(lq.Q); q != "" {
		pat := "%" + likeEscaper.Replace(q) + "%"
		p.Pattern = &pat
	}
	page, err := t.q.ListRecords(t.ctx, p)
	if err != nil {
		return nil, 0, err
	}
	total := page.Total
	ids, err := t.sortIDs(page.Ids)
	if err != nil {
		return nil, 0, err
	}
	ids = ids[min(off, len(ids)):]
	ids = ids[:min(lim, len(ids))]
	ents, err := t.q.ListEntitiesByIDs(t.ctx, sqlc.ListEntitiesByIDsParams{DashboardID: t.Dashboard, Ids: ids})
	if err != nil {
		return nil, 0, err
	}
	byID := map[uuid.UUID]sqlc.Entity{}
	for _, e := range ents {
		byID[e.ID] = e
	}
	out := make([]*Rec, 0, len(ids))
	for _, id := range ids {
		r, err := t.Record(byID[id])
		if errors.Is(err, ErrNotFound) {
			continue // сущность шаблона не загружена
		}
		if err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, nil
}

// createGroups — метки новых записей пакета.
func (t *Tx) createGroups(b Batch, res *BatchResult) error {
	for i, n := range b.Records {
		if len(n.Groups) == 0 {
			continue
		}
		if err := t.setLabels(res.Records[i].Row.ID, n.Groups, fmt.Sprintf("records[%d].groups", i)); err != nil {
			return err
		}
	}
	return nil
}
