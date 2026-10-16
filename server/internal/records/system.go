package records

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"kintion/server/internal/audit"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/packs"
	"kintion/server/internal/partialdate"
)

// Opt — необязательный ключ тела запроса: Set — ключ есть (в том числе null).
type Opt[T any] struct {
	Set bool
	V   *T
}

// UnmarshalJSON отмечает присутствие ключа; null — V = nil.
func (o *Opt[T]) UnmarshalJSON(b []byte) error {
	o.Set = true
	if string(b) == "null" {
		o.V = nil
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	o.V = &v
	return nil
}

// PeriodIn — системная дата во входе: дата и привязка к записи-событию.
// Link — id записи или временный id из пакета.
type PeriodIn struct {
	Value *partialdate.Date `json:"value"`
	Link  *string           `json:"link"`
}

// entitySnap — запись в журнале (системные поля).
type entitySnap struct {
	ID           uuid.UUID         `json:"id"`
	Entity       string            `json:"entity"`
	TemplateID   uuid.UUID         `json:"template_id"`
	StartDate    *partialdate.Date `json:"start_date"`
	StartLink    *uuid.UUID        `json:"start_link"`
	EndDate      *partialdate.Date `json:"end_date"`
	EndLink      *uuid.UUID        `json:"end_link"`
	Note         *string           `json:"note"`
	TitleManual  *string           `json:"title_manual"`
	Sensitive    bool              `json:"sensitive"`
	CustomFields json.RawMessage   `json:"custom_fields"`
	Deleted      bool              `json:"deleted"`
}

func snapEntity(r sqlc.Entity) entitySnap {
	return entitySnap{
		ID: r.ID, Entity: r.Entity, TemplateID: r.TemplateID, StartDate: r.StartDate, StartLink: r.StartLink,
		EndDate: r.EndDate, EndLink: r.EndLink, Note: r.Note, TitleManual: r.TitleManual, Sensitive: r.Sensitive,
		CustomFields: r.CustomFields, Deleted: r.DeletedAt != nil,
	}
}

func (t *Tx) auditEntity(op string, before, after *sqlc.Entity) error {
	e := audit.Entry{Object: "entity", Op: op}
	var id uuid.UUID
	if before != nil {
		id = before.ID
		e.Before = snapEntity(*before)
	}
	if after != nil {
		id = after.ID
		e.After = snapEntity(*after)
	}
	e.Entity, e.ObjectID = &id, &id
	return t.audit(e)
}

// checkPeriod — системная дата key (start / end) допустима для шаблона:
// "end": null в period сущности или шаблона — у записи нет конца (F-70).
func checkPeriod(tm *Tmpl, key, field string) error {
	if _, ok := packs.PeriodLabel(tm.Entity, tm.T, key); !ok {
		return invalid(field, "у записей шаблона нет этой даты")
	}
	return nil
}

// periodLink — цель привязки системной даты: видимая запись дашборда, не сама запись.
func (t *Tx) periodLink(self uuid.UUID, link uuid.UUID, field string) error {
	if link == self {
		return invalid(field, "запись не может быть привязана к себе")
	}
	return t.anchorTarget(link, field)
}

// SystemPatch — PATCH /api/records/{id}: одно системное поле за запрос с
// версией if_rev (F-45).
type SystemPatch struct {
	IfRev        *int64                 `json:"if_rev"`
	Start        Opt[PeriodIn]          `json:"start"`
	End          Opt[PeriodIn]          `json:"end"`
	Note         Opt[string]            `json:"note"`
	TitleManual  Opt[string]            `json:"title_manual"`
	Sensitive    Opt[bool]              `json:"sensitive"`
	CustomFields Opt[[]json.RawMessage] `json:"custom_fields"`
}

// key — ключ версии единственного поля запроса.
func (p SystemPatch) key() (string, error) {
	var keys []string
	for k, set := range map[string]bool{
		RevStart: p.Start.Set, RevEnd: p.End.Set, RevNote: p.Note.Set, RevTitle: p.TitleManual.Set,
		RevSensitive: p.Sensitive.Set, RevCustom: p.CustomFields.Set,
	} {
		if set {
			keys = append(keys, k)
		}
	}
	if len(keys) != 1 {
		return "", invalid("", "запрос меняет ровно одно системное поле")
	}
	return keys[0], nil
}

// systemValue — текущее значение системного поля (ответ 409).
type systemValue struct {
	Rev   int64 `json:"rev"`
	Value any   `json:"value"`
}

func (t *Tx) systemCurrent(r *Rec, key string) func() (any, error) {
	return func() (any, error) {
		row, err := t.q.GetEntityByID(t.ctx, r.Row.ID)
		if err != nil {
			return nil, err
		}
		rev, err := t.rev(row.ID, key)
		if err != nil {
			return nil, err
		}
		var v any
		switch key {
		case RevStart:
			v = period(row.StartDate, row.StartLink)
		case RevEnd:
			v = period(row.EndDate, row.EndLink)
		case RevNote:
			v = row.Note
		case RevTitle:
			v = row.TitleManual
		case RevSensitive:
			v = row.Sensitive
		case RevCustom:
			v = json.RawMessage(row.CustomFields)
		}
		return systemValue{Rev: rev, Value: v}, nil
	}
}

// PatchSystem меняет одно системное поле записи.
func (t *Tx) PatchSystem(r *Rec, p SystemPatch) (*Rec, error) {
	if err := live(r); err != nil {
		return nil, err
	}
	key, err := p.key()
	if err != nil {
		return nil, err
	}
	if err := t.lockRev(r.Row.ID, key, p.IfRev, t.systemCurrent(r, key)); err != nil {
		return nil, err
	}
	row := r.Row
	set := func(pi *PeriodIn, date **partialdate.Date, link **uuid.UUID, field string) error {
		*date, *link = nil, nil
		if pi == nil || (pi.Value == nil && pi.Link == nil) {
			return nil
		}
		if err := checkPeriod(r.Tmpl, field, field); err != nil {
			return err
		}
		*date = pi.Value
		if pi.Link != nil {
			id, err := uuid.Parse(*pi.Link)
			if err != nil {
				return invalid(field+".link", "неверный id записи")
			}
			if err := t.periodLink(row.ID, id, field+".link"); err != nil {
				return err
			}
			*link = &id
		}
		return nil
	}
	switch key {
	case RevStart:
		err = set(p.Start.V, &row.StartDate, &row.StartLink, RevStart)
	case RevEnd:
		err = set(p.End.V, &row.EndDate, &row.EndLink, RevEnd)
	case RevNote:
		row.Note = normText(p.Note.V)
	case RevTitle:
		row.TitleManual = normText(p.TitleManual.V)
		if row.TitleManual != nil && !r.T.FreeEdit() {
			err = invalid("title_manual", "шаблон не разрешает ручную подпись")
		}
	case RevSensitive:
		if p.Sensitive.V == nil {
			err = invalid("sensitive", "нужно да или нет")
		} else {
			row.Sensitive = *p.Sensitive.V
		}
	case RevCustom:
		var list []json.RawMessage
		if p.CustomFields.V != nil {
			list = *p.CustomFields.V
		}
		row.CustomFields, err = t.customFields(r, list)
	}
	if err != nil {
		return nil, err
	}
	nr, err := t.q.UpdateEntitySystem(t.ctx, sqlc.UpdateEntitySystemParams{
		ID: row.ID, StartDate: row.StartDate, StartLink: row.StartLink, EndDate: row.EndDate, EndLink: row.EndLink,
		Note: row.Note, TitleManual: row.TitleManual, Sensitive: row.Sensitive, CustomFields: row.CustomFields,
	})
	if err != nil {
		return nil, err
	}
	if _, err := t.bumpRev(row.ID, key); err != nil {
		return nil, err
	}
	if err := t.auditEntity(audit.OpUpdate, &r.Row, &nr); err != nil {
		return nil, err
	}
	return &Rec{Row: nr, Tmpl: r.Tmpl}, nil
}

// NewRecord — запись в пакетной операции. Ref — временный id: на него
// ссылаются другие элементы пакета (участники, привязки, ref).
type NewRecord struct {
	Ref          string               `json:"ref"`
	Entity       string               `json:"entity"`
	Template     string               `json:"template"`
	Start        *PeriodIn            `json:"start"`
	End          *PeriodIn            `json:"end"`
	Note         *string              `json:"note"`
	TitleManual  *string              `json:"title_manual"`
	Sensitive    *bool                `json:"sensitive"`
	CustomFields []json.RawMessage    `json:"custom_fields"`
	Fields       map[string][]ValueIn `json:"fields"`
	Groups       []uuid.UUID          `json:"groups"`
}

// Batch — пакетная операция: записи (и их значения, участники) одной
// транзакцией и одним operation_id.
type Batch struct {
	Records      []NewRecord      `json:"records"`
	Participants []NewParticipant `json:"participants"`
	// Strict — ошибка значения отменяет весь пакет (новая запись из
	// черновика карточки, F-88: запись появляется только вместе со значением).
	Strict bool `json:"strict"`
}

// FieldError — значение поля, не сохранённое при создании записи (F-15):
// запись сохранена, поле — нет.
type FieldError struct {
	Ref     string `json:"ref"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

// BatchResult — итог пакета.
type BatchResult struct {
	IDs     map[string]uuid.UUID
	Records []*Rec
	Errors  []FieldError
}

// batchRefs — разрешение ссылок пакета: временный id или id записи дашборда.
type batchRefs struct {
	t   *Tx
	ids map[string]uuid.UUID
}

func (b *batchRefs) resolve(s, field string) (uuid.UUID, error) {
	if id, ok := b.ids[s]; ok {
		return id, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, invalid(field, "нет записи %q в пакете", s)
	}
	if _, err := b.t.target(id, field); err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

// templateFor — действующий шаблон новой записи: явный или шаблон по
// умолчанию сущности (F-62).
func (t *Tx) templateFor(n NewRecord, field string) (*Tmpl, error) {
	key := n.Template
	if key == "" {
		if n.Entity == "" {
			return nil, invalid(field+".entity", "нужна сущность или шаблон")
		}
		dt, ok := t.s.Reg.DefaultTemplate(n.Entity)
		if !ok {
			return nil, invalid(field+".entity", "у сущности %q нет шаблона по умолчанию", n.Entity)
		}
		key = dt.ID
	}
	row, err := t.q.FindActiveTemplate(t.ctx, sqlc.FindActiveTemplateParams{DashboardID: t.Dashboard, TemplateKey: key})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, invalid(field+".template", "шаблона %q нет в дашборде", key)
	}
	if err != nil {
		return nil, err
	}
	tm := t.tmpls[row.ID]
	if tm == nil {
		return nil, invalid(field+".template", "шаблон %q не работает: его пак не загружен", key)
	}
	if n.Entity != "" && n.Entity != tm.T.Entity {
		return nil, invalid(field+".entity", "шаблон %q — не для сущности %q", key, n.Entity)
	}
	return tm, nil
}

// Create выполняет пакет: создаёт записи, затем их значения и участников.
// Ошибка записи или участника отменяет пакет; ошибка значения поля — нет
// (F-15): значение пропускается и попадает в Errors.
func (t *Tx) Create(b Batch, account uuid.UUID) (*BatchResult, error) {
	if len(b.Records) == 0 {
		return nil, invalid("records", "пакет без записей")
	}
	res := &BatchResult{IDs: map[string]uuid.UUID{}}
	refs := &batchRefs{t: t, ids: res.IDs}
	tmpls := make([]*Tmpl, len(b.Records))
	for i, n := range b.Records {
		field := fmt.Sprintf("records[%d]", i)
		if n.Ref != "" {
			if _, dup := res.IDs[n.Ref]; dup {
				return nil, invalid(field+".ref", "временный id %q повторяется", n.Ref)
			}
			if _, err := uuid.Parse(n.Ref); err == nil {
				return nil, invalid(field+".ref", "временный id не может быть UUID")
			}
			res.IDs[n.Ref] = newID()
		}
		tm, err := t.templateFor(n, field)
		if err != nil {
			return nil, err
		}
		tmpls[i] = tm
	}
	// 1. записи без привязок дат (цели могут быть ниже в пакете)
	for i, n := range b.Records {
		field := fmt.Sprintf("records[%d]", i)
		tm := tmpls[i]
		id, ok := res.IDs[n.Ref]
		if !ok {
			id = newID()
		}
		p := sqlc.InsertEntityParams{
			ID: id, DashboardID: t.Dashboard, Entity: tm.T.Entity, TemplateID: tm.Row.ID,
			Note: normText(n.Note), TitleManual: normText(n.TitleManual), Sensitive: tm.T.Sensitive,
			CustomFields: []byte("[]"), CreatedBy: &account,
		}
		if n.Sensitive != nil {
			p.Sensitive = *n.Sensitive
		}
		if p.TitleManual != nil && !tm.T.FreeEdit() {
			return nil, invalid(field+".title_manual", "шаблон не разрешает ручную подпись")
		}
		for _, pd := range []struct {
			in   *PeriodIn
			key  string
			date **partialdate.Date
		}{{n.Start, RevStart, &p.StartDate}, {n.End, RevEnd, &p.EndDate}} {
			if pd.in == nil || (pd.in.Value == nil && pd.in.Link == nil) {
				continue
			}
			if err := checkPeriod(tm, pd.key, field+"."+pd.key); err != nil {
				return nil, err
			}
			*pd.date = pd.in.Value
		}
		row, err := t.q.InsertEntity(t.ctx, p)
		if err != nil {
			return nil, err
		}
		res.Records = append(res.Records, &Rec{Row: row, Tmpl: tm})
	}
	// 2. привязки дат и описания свободных полей
	for i, n := range b.Records {
		field := fmt.Sprintf("records[%d]", i)
		r := res.Records[i]
		row, changed := r.Row, false
		for _, pd := range []struct {
			in   *PeriodIn
			key  string
			link **uuid.UUID
		}{{n.Start, RevStart, &row.StartLink}, {n.End, RevEnd, &row.EndLink}} {
			if pd.in == nil || pd.in.Link == nil {
				continue
			}
			id, err := refs.resolve(*pd.in.Link, field+"."+pd.key+".link")
			if err != nil {
				return nil, err
			}
			if id == row.ID {
				return nil, invalid(field+"."+pd.key+".link", "запись не может быть привязана к себе")
			}
			if err := t.anchorTarget(id, field+"."+pd.key+".link"); err != nil {
				return nil, err
			}
			*pd.link, changed = &id, true
		}
		if len(n.CustomFields) > 0 {
			cf, err := t.customFields(r, n.CustomFields)
			if err != nil {
				var ve *ValidationError
				if errors.As(err, &ve) {
					ve.Field = field + "." + strings.TrimPrefix(ve.Field, ".")
				}
				return nil, err
			}
			row.CustomFields, changed = cf, true
		}
		if changed {
			nr, err := t.q.UpdateEntitySystem(t.ctx, sqlc.UpdateEntitySystemParams{
				ID: row.ID, StartDate: row.StartDate, StartLink: row.StartLink, EndDate: row.EndDate, EndLink: row.EndLink,
				Note: row.Note, TitleManual: row.TitleManual, Sensitive: row.Sensitive, CustomFields: row.CustomFields,
			})
			if err != nil {
				return nil, err
			}
			r.Row = nr
		}
		if err := t.auditEntity(audit.OpCreate, nil, &r.Row); err != nil {
			return nil, err
		}
	}
	if err := t.createRest(b, res, refs); err != nil {
		return nil, err
	}
	return res, nil
}
