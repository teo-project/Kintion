package records

import (
	"encoding/json"
	"slices"
	"strings"

	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/packs"
)

// Свои блоки карточки (F-139): блок записи — в custom_sections записи, общий
// блок шаблона в дашборде — в overrides.custom_sections шаблона дашборда
// (та же область, что у своих полей, F-96). Своё поле ставится в любой блок:
// блок шаблона (section полей шаблона), «Свои поля» или свой блок. Блок, в
// котором есть поля, не удаляется.

// SectionsKey — ключ общих своих блоков в overrides шаблона дашборда.
const SectionsKey = "custom_sections"

// Блоки, которые есть у любой карточки: основной и «Свои поля» (поле без
// section).
const (
	mainSection   = "main"
	customSection = "custom"
)

// customSectionDef — описание своего блока.
type customSectionDef struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// builtinSections — блоки шаблона: основной, «Свои поля» и section его полей.
func builtinSections(tm *Tmpl) map[string]bool {
	out := map[string]bool{mainSection: true, customSection: true}
	if tm != nil && tm.T != nil {
		for _, f := range tm.T.Fields {
			if f.Section != "" {
				out[f.Section] = true
			}
		}
	}
	return out
}

// sectionKeys — ключи своих блоков из хранимого списка.
func sectionKeys(raw []byte) []string {
	var list []customSectionDef
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &list)
	}
	keys := make([]string, 0, len(list))
	for _, s := range list {
		keys = append(keys, s.Key)
	}
	return keys
}

// sharedSectionsRaw — общие свои блоки шаблона дашборда как хранятся.
func sharedSectionsRaw(overrides []byte) json.RawMessage {
	var ov map[string]json.RawMessage
	if len(overrides) > 0 && json.Unmarshal(overrides, &ov) == nil {
		return ov[SectionsKey]
	}
	return nil
}

// sharedSectionKeys — общие свои блоки шаблона дашборда.
func sharedSectionKeys(overrides []byte) []string {
	return sectionKeys(sharedSectionsRaw(overrides))
}

// checkSections — описания своих блоков: ключ (латиница, цифры, _) не
// повторяется и не совпадает с блоками reserved, название непустое.
func checkSections(field string, list []json.RawMessage, reserved map[string]bool) ([]string, error) {
	keys := make([]string, 0, len(list))
	for _, raw := range list {
		var s customSectionDef
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, invalid(field, "неверное описание блока: %v", err)
		}
		switch {
		case !customKeyRe.MatchString(s.Key):
			return nil, invalid(field, "ключ блока %q: латиница в нижнем регистре, цифры, _", s.Key)
		case strings.TrimSpace(s.Name) == "":
			return nil, invalid(field, "блок %q: нужно название", s.Key)
		case reserved[s.Key] || slices.Contains(keys, s.Key):
			return nil, invalid(field, "ключ блока %q уже занят", s.Key)
		}
		keys = append(keys, s.Key)
	}
	return keys, nil
}

// checkFieldSections — каждое своё поле стоит в существующем блоке; поле в
// блоке removed — «блок с полями не удаляется».
func checkFieldSections(field string, defs []packs.Field, allowed map[string]bool, removed []string) error {
	for _, d := range defs {
		if d.Section == "" || allowed[d.Section] {
			continue
		}
		if slices.Contains(removed, d.Section) {
			return invalid(SectionsKey, "в блоке %q есть поля: сначала перенесите или уберите их", d.Section)
		}
		return invalid(field, "поле %q: нет блока %q", d.Key, d.Section)
	}
	return nil
}

func union(sets ...map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, s := range sets {
		for k := range s {
			out[k] = true
		}
	}
	return out
}

func setOf(keys []string) map[string]bool {
	out := make(map[string]bool, len(keys))
	for _, k := range keys {
		out[k] = true
	}
	return out
}

func minus(a, b []string) []string {
	var out []string
	for _, k := range a {
		if !slices.Contains(b, k) {
			out = append(out, k)
		}
	}
	return out
}

// recordSections проверяет свои блоки записи и блоки её своих полей (fields —
// итоговое описание полей): ключ блока не совпадает с блоками шаблона и
// общими блоками; блок с полями не удаляется. list == nil — блоки прежние.
func (t *Tx) recordSections(r *Rec, list *[]json.RawMessage, fields []byte) ([]byte, error) {
	cur := r.Row.CustomSections
	if list != nil {
		cur = nil
		if *list != nil {
			b, err := json.Marshal(*list)
			if err != nil {
				return nil, err
			}
			cur = b
		}
	}
	var items []json.RawMessage
	if len(cur) > 0 {
		if err := json.Unmarshal(cur, &items); err != nil {
			return nil, invalid(SectionsKey, "неверный список блоков")
		}
	}
	defs, err := customDefs(fields)
	if err != nil {
		return nil, err
	}
	shared := sharedSectionKeys(r.Tmpl.Row.Overrides)
	if list != nil || slices.ContainsFunc(defs, func(d packs.Field) bool { return slices.Contains(shared, d.Section) }) {
		// новые блоки записи или поле в общем блоке: строка шаблона блокируется до
		// конца транзакции — одновременная правка общих блоков (PatchShared, FOR
		// UPDATE) дождётся и увидит и ключи блоков записи, и поле в общем блоке
		row, err := t.q.ShareLockDashboardTemplate(t.ctx, sqlc.ShareLockDashboardTemplateParams{DashboardID: t.Dashboard, ID: r.Row.TemplateID})
		if err != nil {
			return nil, err
		}
		shared = sharedSectionKeys(row.Overrides)
	}
	reserved := union(builtinSections(r.Tmpl), setOf(shared))
	keys, err := checkSections(SectionsKey, items, reserved)
	if err != nil {
		return nil, err
	}
	removed := minus(sectionKeys(r.Row.CustomSections), keys)
	if err := checkFieldSections(RevCustom, defs, union(reserved, setOf(keys)), removed); err != nil {
		return nil, err
	}
	if items == nil {
		items = []json.RawMessage{}
	}
	return json.Marshal(items)
}
