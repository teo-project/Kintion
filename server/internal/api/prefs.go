package api

import (
	"encoding/json"
	"net/http"

	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/httpx"
)

// Варианты правки поля с историей на карточке (F-80).
const (
	HistoryEditAsk     = ""
	HistoryEditReplace = "replace"
	HistoryEditAdd     = "add"
)

// Режим карточек аккаунта (F-108).
const (
	CardModeRead   = "read"
	CardModeInline = "inline"
	CardModeForm   = "form"
)

// Prefs — настройки пользователя на сервере: запомненные выборы интерфейса.
type Prefs struct {
	// HistoryEdit — что делать при правке поля с историей (F-80):
	// "replace" — заменить текущее, "add" — добавить новое в историю,
	// "" — спрашивать.
	HistoryEdit string `json:"history_edit"`
	// CardMode — как открываются карточки (F-108): "read" — чтение (по
	// умолчанию), "inline" — правка по месту, "form" — сразу форма.
	CardMode string `json:"card_mode"`
	// TreeRibbon — траурная лента на узлах дерева (F-119); нет — да.
	TreeRibbon *bool `json:"tree_ribbon"`
	// TreeHideDead — «Скрыть умерших» в дереве (F-119).
	TreeHideDead bool `json:"tree_hide_dead"`
}

// withDefaults — незаданное — по умолчанию.
func (p *Prefs) withDefaults() {
	if p.TreeRibbon == nil {
		on := true
		p.TreeRibbon = &on
	}
}

func (a *API) prefsRoutes() {
	a.mux.HandleFunc("GET /api/me/prefs", a.getPrefs)
	a.mux.HandleFunc("PUT /api/me/prefs", a.putPrefs)
}

func readPrefs(raw []byte) Prefs {
	var p Prefs
	_ = json.Unmarshal(raw, &p) // испорченные настройки — как пустые
	switch p.CardMode {
	case CardModeInline, CardModeForm:
	default:
		p.CardMode = CardModeRead
	}
	p.withDefaults()
	return p
}

// getPrefs — GET /api/me/prefs.
func (a *API) getPrefs(w http.ResponseWriter, r *http.Request) {
	sess, _ := SessionFrom(r.Context())
	raw, err := sqlc.New(a.d.Pool).GetAccountPrefs(r.Context(), sess.Account.ID)
	if err != nil {
		a.internal(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, readPrefs(raw))
}

// putPrefs — PUT /api/me/prefs: настройки целиком.
func (a *API) putPrefs(w http.ResponseWriter, r *http.Request) {
	var in Prefs
	if !decode(w, r, &in) {
		return
	}
	switch in.HistoryEdit {
	case HistoryEditAsk, HistoryEditReplace, HistoryEditAdd:
	default:
		httpx.WriteJSON(w, http.StatusBadRequest, ValidationBody{Error: "validation", Field: "history_edit", Message: "только replace, add или пусто"})
		return
	}
	switch in.CardMode {
	case "":
		in.CardMode = CardModeRead // старый клиент без режима — по умолчанию
	case CardModeRead, CardModeInline, CardModeForm:
	default:
		httpx.WriteJSON(w, http.StatusBadRequest, ValidationBody{Error: "validation", Field: "card_mode", Message: "только read, inline или form"})
		return
	}
	in.withDefaults()
	sess, _ := SessionFrom(r.Context())
	raw, err := json.Marshal(in)
	if err != nil {
		a.internal(w, r, err)
		return
	}
	out, err := sqlc.New(a.d.Pool).UpdateAccountPrefs(r.Context(), sqlc.UpdateAccountPrefsParams{ID: sess.Account.ID, Prefs: raw})
	if err != nil {
		a.internal(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, readPrefs(out))
}
