package api_test

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"kintion/server/internal/auth"
)

// guest — второй аккаунт: админ (владелец) заводит его с ником, клиент входит.
func guest(t *testing.T, e *env, owner *client, login, nick string) *client {
	t.Helper()
	expect(t, owner.do("POST", "/api/admin/accounts", map[string]any{"login": login, "password": "guest-pass-1", "nick": nick}),
		http.StatusCreated, "create account "+login)
	g := e.client()
	expect(t, g.do("POST", "/api/login", map[string]string{"login": login, "password": "guest-pass-1"}), http.StatusOK, "login "+login)
	return g
}

// share — владелец выдаёт ник право на группу; гость принимает приглашение.
func share(t *testing.T, owner, g *client, dash, nick, grp string, edit bool) {
	t.Helper()
	expect(t, owner.do("PUT", "/api/dashboards/"+dash+"/grants", map[string]any{"nick": nick, "group_id": grp, "can_edit": edit}),
		http.StatusOK, "grant")
	if inv := g.doList("/api/invitations"); len(inv) == 1 {
		expect(t, g.do("POST", "/api/invitations/"+dash+"/accept", nil), http.StatusNoContent, "accept")
	}
}

// F-28: режим invite — регистрация только по ссылке админа, одноразовой;
// open — без приглашения; closed — никак. Ник уникален без учёта регистра.
func TestRegistration(t *testing.T) {
	e := newEnv(t)
	owner := e.setupOwner()
	r := e.client().do("GET", "/api/register", nil)
	if r.body["mode"] != auth.RegistrationInvite {
		t.Fatalf("mode %v", r.body)
	}
	expect(t, e.client().do("POST", "/api/register", map[string]any{"login": "x", "password": "secret-pass"}),
		http.StatusForbidden, "no invite")
	expect(t, e.client().do("POST", "/api/invites", nil), http.StatusUnauthorized, "invite without session")

	inv := owner.do("POST", "/api/invites", nil)
	expect(t, inv, http.StatusCreated, "invite")
	token := inv.body["token"].(string)
	if r := e.client().do("GET", "/api/register?token="+url.QueryEscape(token), nil); r.body["invite_valid"] != true {
		t.Fatalf("invite not valid %v", r.body)
	}
	if l := owner.doList("/api/invites"); len(l) != 1 || l[0]["token"] != nil {
		t.Fatalf("invites list (no token) %v", l)
	}
	expect(t, owner.do("PUT", "/api/me/nick", map[string]any{"nick": "Papa"}), http.StatusOK, "owner nick")
	expect(t, e.client().do("POST", "/api/register", map[string]any{"token": token, "login": "wife", "password": "secret-pass", "nick": "papa"}),
		http.StatusBadRequest, "nick taken, other case")
	wife := e.client()
	r = wife.do("POST", "/api/register", map[string]any{"token": token, "login": "wife", "password": "secret-pass", "nick": "mama"})
	expect(t, r, http.StatusCreated, "register")
	if acc := r.body["account"].(map[string]any); acc["is_admin"] != false || acc["nick"] != "mama" {
		t.Fatalf("account %v", acc)
	}
	if len(wife.doList("/api/dashboards")) != 1 {
		t.Fatal("registered account has no own dashboard")
	}
	expect(t, e.client().do("POST", "/api/register", map[string]any{"token": token, "login": "other", "password": "secret-pass"}),
		http.StatusForbidden, "invite used twice")
	// админские маршруты не-админу не видны; в invite приглашает только админ
	expect(t, wife.do("GET", "/api/admin/accounts", nil), http.StatusNotFound, "admin route for non-admin")
	expect(t, wife.do("POST", "/api/invites", nil), http.StatusNotFound, "invite by non-admin in invite mode")
	if l := owner.doList("/api/admin/accounts"); len(l) != 2 {
		t.Fatalf("accounts %v", l)
	}
	// аккаунт, заведённый админом: автор в журнале — админ
	expect(t, owner.do("POST", "/api/admin/accounts", map[string]any{"login": "son", "password": "secret-pass"}), http.StatusCreated, "admin creates")
	var actor string
	if err := e.pool.QueryRow(context.Background(),
		`SELECT a.login FROM audit_log l JOIN account a ON a.id = l.actor_id WHERE l.object = 'account' AND l.after->>'login' = 'son'`).Scan(&actor); err != nil || actor != "owner" {
		t.Fatalf("account create actor %q %v", actor, err)
	}

	envRegistration = auth.RegistrationOpen
	t.Cleanup(func() { envRegistration = auth.RegistrationInvite })
	open := newEnv(t)
	// до первичной настройки регистрации нет: иначе владельца не создать
	expect(t, open.client().do("POST", "/api/register", map[string]any{"login": "early", "password": "secret-pass"}),
		http.StatusForbidden, "register before setup")
	open.setupOwner()
	expect(t, open.client().do("POST", "/api/register", map[string]any{"login": "anyone", "password": "secret-pass"}),
		http.StatusCreated, "open registration")

	// invite-all: приглашает любой; видит и удаляет только свои приглашения
	envRegistration = auth.RegistrationInviteAll
	all := newEnv(t)
	allOwner := all.setupOwner()
	expect(t, allOwner.do("POST", "/api/admin/accounts", map[string]any{"login": "u1", "password": "secret-pass"}), http.StatusCreated, "u1")
	u1 := all.client()
	expect(t, u1.do("POST", "/api/login", map[string]string{"login": "u1", "password": "secret-pass"}), http.StatusOK, "u1 login")
	adminInv := allOwner.do("POST", "/api/invites", nil)
	expect(t, adminInv, http.StatusCreated, "admin invite")
	userInv := u1.do("POST", "/api/invites", nil)
	expect(t, userInv, http.StatusCreated, "user invite")
	if l := u1.doList("/api/invites"); len(l) != 1 || l[0]["id"] != userInv.body["id"] {
		t.Fatalf("user sees not only own invites %v", l)
	}
	if l := allOwner.doList("/api/invites"); len(l) != 2 {
		t.Fatalf("admin sees all invites %v", l)
	}
	expect(t, u1.do("DELETE", "/api/invites/"+adminInv.body["id"].(string), nil), http.StatusNoContent, "foreign invite delete is a no-op")
	if l := allOwner.doList("/api/invites"); len(l) != 2 {
		t.Fatal("user deleted a foreign invite")
	}
	expect(t, all.client().do("POST", "/api/register", map[string]any{"token": userInv.body["token"], "login": "u2", "password": "secret-pass"}),
		http.StatusCreated, "register by user invite")

	envRegistration = auth.RegistrationClosed
	closed := newEnv(t)
	closed.setupOwner()
	expect(t, closed.client().do("POST", "/api/register", map[string]any{"login": "anyone", "password": "secret-pass"}),
		http.StatusForbidden, "closed registration")
}

// F-03, F-05, F-07, F-105: гость видит только записи групп с выдачей и
// вложенных в них; невидимое ведёт себя как несуществующее — в списке, счётчиках,
// карточке, дереве, связях; ждущее приглашение доступа не даёт.
func TestGuestSeesOnlyGrantedGroups(t *testing.T) {
	e := newEnv(t)
	owner := e.setupOwner()
	dash := dashboardOf(t, owner)
	family := group(t, owner, dash, "Наша семья")
	ivanov := group(t, owner, dash, "Ивановы", family)
	private := group(t, owner, dash, "Личное")

	r := batch(t, owner, dash, map[string]any{
		"records": []any{
			map[string]any{"ref": "husband", "entity": "kin.core.person", "groups": []any{ivanov},
				"fields": map[string]any{"name": []any{map[string]any{"value": "Иван"}}}},
			map[string]any{"ref": "son", "entity": "kin.core.person", "groups": []any{family},
				"fields": map[string]any{"name": []any{map[string]any{"value": "Петя"}}}},
			map[string]any{"ref": "julia", "entity": "kin.core.person", "groups": []any{private},
				"fields": map[string]any{"name": []any{map[string]any{"value": "Юлия"}}}},
			map[string]any{"ref": "love", "template": "kin.core.relationship", "groups": []any{family}},
			map[string]any{"ref": "secret", "template": "kin.core.relationship", "groups": []any{private}},
			map[string]any{"ref": "pc", "template": "kin.core.parent_child", "groups": []any{family}},
		},
		"participants": []any{
			map[string]any{"container": "love", "entity": "husband", "role": "partner"},
			map[string]any{"container": "love", "entity": "julia", "role": "partner"},
			map[string]any{"container": "secret", "entity": "husband", "role": "partner"},
			map[string]any{"container": "secret", "entity": "julia", "role": "partner"},
			map[string]any{"container": "pc", "entity": "husband", "role": "parent"},
			map[string]any{"container": "pc", "entity": "son", "role": "child"},
		},
	}, http.StatusCreated)
	id := func(ref string) string { return created(t, r, ref) }

	wife := guest(t, e, owner, "wife", "mama")
	// нет такого ника — нет (F-03); себе — нельзя
	expect(t, owner.do("PUT", "/api/dashboards/"+dash+"/grants", map[string]any{"nick": "nobody", "group_id": family}),
		http.StatusBadRequest, "unknown nick")
	expect(t, wife.do("GET", "/api/dashboards/"+dash+"/groups", nil), http.StatusNotFound, "no access before grant")

	expect(t, owner.do("PUT", "/api/dashboards/"+dash+"/grants", map[string]any{"nick": "MAMA", "group_id": family}),
		http.StatusOK, "grant by nick, any case")
	// приглашение ждёт подтверждения — доступа ещё нет
	inv := wife.doList("/api/invitations")
	if len(inv) != 1 || inv[0]["dashboard_id"] != dash || inv[0]["name"] == "" {
		t.Fatalf("invitations %v", inv)
	}
	expect(t, wife.do("GET", "/api/records/"+id("husband"), nil), http.StatusNotFound, "pending invitation")
	expect(t, wife.do("POST", "/api/invitations/"+dash+"/accept", nil), http.StatusNoContent, "accept")
	if inv := wife.doList("/api/invitations"); len(inv) != 0 {
		t.Fatalf("invitation stays %v", inv)
	}
	ds := wife.doList("/api/dashboards")
	if len(ds) != 2 || ds[0]["guest"] == true || ds[1]["id"] != dash || ds[1]["guest"] != true {
		t.Fatalf("dashboards %v", ds)
	}
	// дальнейшие выдачи в тот же дашборд — без подтверждения
	other := group(t, owner, dash, "Соседи")
	expect(t, owner.do("PUT", "/api/dashboards/"+dash+"/grants", map[string]any{"nick": "mama", "group_id": other}), http.StatusOK, "second grant")
	if inv := wife.doList("/api/invitations"); len(inv) != 0 {
		t.Fatalf("second grant asks again %v", inv)
	}

	// список: «Наша семья» и вложенные «Ивановы» (F-105), без «Личного»
	got := list(t, wife, dash, url.Values{})
	want := []string{id("husband"), id("son"), id("love"), id("pc")}
	if len(got) != len(want) {
		t.Fatalf("guest list %v, want %v", got, want)
	}
	for _, w := range want {
		if !got[w] {
			t.Fatalf("guest list misses %s: %v", w, got)
		}
	}
	if got := list(t, wife, dash, url.Values{"q": {"Юлия"}}); len(got) != 0 {
		t.Fatalf("search finds invisible %v", got)
	}
	if got := list(t, wife, dash, url.Values{"deleted": {"1"}}); len(got) != 0 {
		t.Fatalf("guest trash %v", got)
	}
	expect(t, wife.do("GET", "/api/dashboards/"+dash+"/records?group="+private, nil), http.StatusBadRequest, "invisible group filter as missing")

	// группы: видимые, счётчики и связи — по видимому
	gs := groupsOf(t, wife, dash)
	if len(gs) != 3 || gs[private] != nil {
		t.Fatalf("guest groups %v", gs)
	}
	if gs[family]["count"] != float64(3) || gs[ivanov]["count"] != float64(1) {
		t.Fatalf("counts %v %v", gs[family]["count"], gs[ivanov]["count"])
	}

	// карточка: невидимая — 404 как несуществующая; у видимой — без невидимых
	// соседей, связей и меток (F-05)
	expect(t, wife.do("GET", "/api/records/"+id("julia"), nil), http.StatusNotFound, "invisible record")
	expect(t, wife.do("GET", "/api/records/"+id("secret"), nil), http.StatusNotFound, "invisible relation")
	h := wife.do("GET", "/api/records/"+id("husband"), nil)
	expect(t, h, http.StatusOK, "visible record")
	rec := record(h)
	var in []string
	for _, p := range rec["participates_in"].([]any) {
		in = append(in, p.(map[string]any)["container_id"].(string))
	}
	if slices.Contains(in, id("secret")) || !slices.Contains(in, id("love")) {
		t.Fatalf("participates_in %v", in)
	}
	if rel := h.body["related"].(map[string]any); rel[id("julia")] != nil || rel[id("secret")] != nil {
		t.Fatalf("related leaks %v", rel)
	}
	love := record(wife.do("GET", "/api/records/"+id("love"), nil))
	if ps := love["participants"].([]any); len(ps) != 1 {
		t.Fatalf("invisible participant shown %v", ps)
	}
	for _, g := range rec["groups"].([]any) {
		if g == private {
			t.Fatal("invisible group label shown")
		}
	}
	// метка невидимой группы у гостя не видна (версия меток — общая, F-147)
	ownRec := record(owner.do("GET", "/api/records/"+id("husband"), nil))
	gRev := ownRec["revs"].(map[string]any)["groups"]
	expect(t, owner.do("PUT", "/api/records/"+id("husband")+"/groups", map[string]any{"if_rev": gRev, "groups": []string{ivanov, private}}),
		http.StatusOK, "hidden label")
	after := record(wife.do("GET", "/api/records/"+id("husband"), nil))
	if len(after["groups"].([]any)) != 1 {
		t.Fatalf("hidden label leaks: groups %v", after["groups"])
	}
	// значение-ссылка на невидимую запись гостю не показывается вовсе (F-05, F-42)
	ownRec = record(owner.do("GET", "/api/records/"+id("husband"), nil))
	expect(t, owner.do("PATCH", "/api/records/"+id("husband"), map[string]any{"if_rev": ownRec["revs"].(map[string]any)["custom_fields"],
		"custom_fields": []any{map[string]any{"key": "friend", "type": "ref", "name": "Друг", "multiple": true}}}), http.StatusOK, "ref field")
	expect(t, owner.do("POST", "/api/records/"+id("husband")+"/fields/custom.friend/values", map[string]any{"if_rev": 0, "value": id("julia")}),
		http.StatusOK, "hidden ref")
	expect(t, owner.do("POST", "/api/records/"+id("husband")+"/fields/custom.friend/values", map[string]any{"if_rev": 1, "value": id("son")}),
		http.StatusOK, "visible ref")
	guestRec := record(wife.do("GET", "/api/records/"+id("husband"), nil))
	if vs := values(guestRec, "custom.friend"); len(vs) != 1 || vs[0]["value"] != id("son") {
		t.Fatalf("guest ref values %v", vs)
	}

	// дерево: родитель — ребёнок, без отношения с невидимой
	rels, recs := treeIDs(t, wife, "/api/dashboards/"+dash+"/tree")
	if len(rels) != 1 || rels[0] != id("pc") || len(recs) != 2 {
		t.Fatalf("guest tree rels %v recs %v", rels, recs)
	}

	// с правом просмотра гость видимое не меняет и не создаёт (F-148): 403 — запись ему видна
	expect(t, wife.do("PATCH", "/api/records/"+id("husband"), map[string]any{"note": "x", "if_rev": 0}), http.StatusForbidden, "guest edit")
	batch(t, wife, dash, map[string]any{"records": []any{map[string]any{"ref": "p", "entity": "kin.core.person"}}}, http.StatusForbidden)
	expect(t, wife.do("GET", "/api/dashboards/"+dash+"/members", nil), http.StatusNotFound, "members for guest")

	// журнал гостя — только свои действия (F-08): принятие приглашения
	page := auditPageOf(t, wife, dash, url.Values{})
	for _, o := range page.Operations {
		if o.ActorLogin != "wife" {
			t.Fatalf("guest sees foreign audit %+v", o)
		}
	}
	if len(page.Operations) != 1 {
		t.Fatalf("guest audit ops %d", len(page.Operations))
	}

	// приглашение и принятие — в журнале владельца
	var created, accepted bool
	for _, o := range auditPageOf(t, owner, dash, url.Values{}).Operations {
		for _, en := range o.Entries {
			created = created || (en.Object == "dashboard_member" && en.Op == "create")
			accepted = accepted || (en.Object == "dashboard_member" && en.Op == "update")
		}
	}
	if !created || !accepted {
		t.Fatalf("member audit: created %v accepted %v", created, accepted)
	}

	// отзыв права и уход гостя; третье право — на техническую группу гостя (F-149)
	ms := owner.doList("/api/dashboards/" + dash + "/members")
	if len(ms) != 1 || ms[0]["accepted"] != true || len(ms[0]["grants"].([]any)) != 3 {
		t.Fatalf("members %v", ms)
	}
	acc := ms[0]["account_id"].(string)
	expect(t, owner.do("DELETE", "/api/dashboards/"+dash+"/grants/"+family+"/"+acc, nil), http.StatusOK, "revoke")
	expect(t, wife.do("GET", "/api/records/"+id("husband"), nil), http.StatusNotFound, "after revoke")
	expect(t, wife.do("DELETE", "/api/dashboards/"+dash+"/membership", nil), http.StatusNoContent, "leave")
	if len(wife.doList("/api/dashboards")) != 1 || len(owner.doList("/api/dashboards/"+dash+"/members")) != 0 {
		t.Fatal("guest still a member")
	}
}

// Отказ от приглашения удаляет и права; следующее приглашение снова ждёт подтверждения.
func TestDeclineInvitation(t *testing.T) {
	e := newEnv(t)
	owner := e.setupOwner()
	dash := dashboardOf(t, owner)
	g := group(t, owner, dash, "Семья")
	wife := guest(t, e, owner, "wife", "mama")
	expect(t, owner.do("PUT", "/api/dashboards/"+dash+"/grants", map[string]any{"nick": "mama", "group_id": g}), http.StatusOK, "grant")
	expect(t, wife.do("POST", "/api/invitations/"+dash+"/decline", nil), http.StatusNoContent, "decline")
	if len(owner.doList("/api/dashboards/"+dash+"/members")) != 0 {
		t.Fatal("declined member stays")
	}
	share(t, owner, wife, dash, "mama", g, false)
	expect(t, wife.do("GET", "/api/dashboards/"+dash+"/groups", nil), http.StatusOK, "after accept")

	// удаление группы снимает права на неё — и это в журнале (F-16)
	expect(t, owner.do("DELETE", "/api/dashboards/"+dash+"/groups/"+g, nil), http.StatusNoContent, "delete group")
	revoked := false
	for _, o := range auditPageOf(t, owner, dash, url.Values{}).Operations {
		for _, en := range o.Entries {
			revoked = revoked || (en.Object == "grp_grant" && en.Op == "delete")
		}
	}
	if !revoked {
		t.Fatal("grant removal by group delete not in audit")
	}
}

// Гостю файл отдаётся, только если он в значении поля видимой записи (F-30).
func TestGuestFiles(t *testing.T) {
	e := newEnv(t)
	owner := e.setupOwner()
	dash := dashboardOf(t, owner)
	fam, priv := group(t, owner, dash, "Семья"), group(t, owner, dash, "Личное")
	shaVisible := uploadPNG(t, owner, dash, 3)
	shaHidden := uploadPNG(t, owner, dash, 4)
	person(t, owner, dash, map[string]any{"groups": []any{fam}, "fields": map[string]any{"photo": []any{map[string]any{"value": shaVisible}}}})
	person(t, owner, dash, map[string]any{"groups": []any{priv}, "fields": map[string]any{"photo": []any{map[string]any{"value": shaHidden}}}})
	wife := guest(t, e, owner, "wife", "mama")
	share(t, owner, wife, dash, "mama", fam, false)
	expect(t, wife.do("GET", "/api/files/"+dash+"/"+shaVisible, nil), http.StatusOK, "visible file")
	expect(t, wife.do("GET", "/api/files/"+dash+"/"+shaHidden, nil), http.StatusNotFound, "hidden file")
	expect(t, owner.do("GET", "/api/files/"+dash+"/"+shaHidden, nil), http.StatusOK, "owner file")
	// хеш скрытого файла в текстовом поле видимой записи доступа не даёт
	person(t, owner, dash, map[string]any{"groups": []any{fam}, "fields": map[string]any{"nickname": []any{map[string]any{"value": shaHidden}}}})
	expect(t, wife.do("GET", "/api/files/"+dash+"/"+shaHidden, nil), http.StatusNotFound, "hidden file by text value")
	expect(t, wife.do("GET", "/api/files/"+dash+"/"+shaHidden+"/preview?shape=square&size=64", nil), http.StatusNotFound, "hidden preview")
	// повторная загрузка скрытого файла гостем неотличима от новой (F-07)
	share(t, owner, wife, dash, "mama", fam, true)
	m := image.NewRGBA(image.Rect(0, 0, 8, 8))
	m.Set(0, 0, color.RGBA{R: 4, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, m); err != nil {
		t.Fatal(err)
	}
	if st, f := wife.upload(dash, buf.Bytes()); st != http.StatusCreated || f["sha256"] != shaHidden {
		t.Fatalf("re-upload of hidden file %d %v", st, f)
	}
	// известный хеш скрытого файла гостю доступа не даёт; загруженный им
	// самим (и повторно — скрытый) прикрепляется
	own := person(t, owner, dash, map[string]any{"groups": []any{fam}})
	photo := func(sha string) resp {
		return wife.do("POST", "/api/records/"+own+"/fields/photo/values", map[string]any{"if_rev": fieldRev(record(wife.do("GET", "/api/records/"+own, nil)), "photo"), "value": sha})
	}
	shaSecret := uploadPNG(t, owner, dash, 5)
	person(t, owner, dash, map[string]any{"groups": []any{priv}, "fields": map[string]any{"photo": []any{map[string]any{"value": shaSecret}}}})
	expect(t, photo(shaSecret), http.StatusBadRequest, "attach hidden file by hash")
	expect(t, wife.do("GET", "/api/files/"+dash+"/"+shaSecret, nil), http.StatusNotFound, "hidden file after attach attempt")
	expect(t, photo(shaHidden), http.StatusOK, "attach re-uploaded file")
	// загрузок в журнале гостя нет — и страница не пустеет от них
	uploadPNG(t, wife, dash, 11)
	page := auditPageOf(t, wife, dash, url.Values{"limit": {"1"}})
	if len(page.Operations) != 1 || page.Operations[0].Entries[0].Object == "file" {
		t.Fatalf("guest audit first page %+v", page.Operations)
	}
}

// uploadPNG загружает PNG, разный для разных seed; возвращает sha256.
func uploadPNG(t *testing.T, c *client, dash string, seed uint8) string {
	t.Helper()
	m := image.NewRGBA(image.Rect(0, 0, 8, 8))
	m.Set(0, 0, color.RGBA{R: seed, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, m); err != nil {
		t.Fatal(err)
	}
	st, f := c.upload(dash, buf.Bytes())
	if st != http.StatusCreated {
		t.Fatalf("upload %d %v", st, f)
	}
	return f["sha256"].(string)
}

// F-148, F-149: гость-редактор правит записи групп с правкой, создаёт записи —
// они попадают в его невидимую ему техническую группу и остаются доступны на
// правку; метки — только групп с правкой, остальные сохраняются; удаление —
// у владельца; гость только с просмотром не создаёт.
func TestGuestEditor(t *testing.T) {
	e := newEnv(t)
	owner := e.setupOwner()
	dash := dashboardOf(t, owner)
	fam, ro, priv := group(t, owner, dash, "Семья"), group(t, owner, dash, "Соседи"), group(t, owner, dash, "Личное")
	r := batch(t, owner, dash, map[string]any{"records": []any{
		map[string]any{"ref": "a", "entity": "kin.core.person", "groups": []any{fam}},
		map[string]any{"ref": "b", "entity": "kin.core.person", "groups": []any{ro}},
		map[string]any{"ref": "h", "entity": "kin.core.person", "groups": []any{priv}},
		map[string]any{"ref": "c", "entity": "kin.core.person", "groups": []any{ro}},
		map[string]any{"ref": "rel", "template": "kin.core.friend", "groups": []any{ro}},
	}, "participants": []any{
		map[string]any{"container": "rel", "entity": "b", "role": "friend"},
		map[string]any{"container": "rel", "entity": "c", "role": "friend"},
	}}, http.StatusCreated)
	id := func(ref string) string { return created(t, r, ref) }
	wife := guest(t, e, owner, "wife", "mama")
	share(t, owner, wife, dash, "mama", fam, true)
	share(t, owner, wife, dash, "mama", ro, false)

	if ds := wife.doList("/api/dashboards"); ds[1]["can_edit"] != true {
		t.Fatalf("can_edit %v", ds)
	}
	gs := groupsOf(t, wife, dash)
	if gs[fam]["editable"] != true || gs[ro]["editable"] != false {
		t.Fatalf("group editable flags %v %v", gs[fam], gs[ro])
	}
	rec := record(wife.do("GET", "/api/records/"+id("a"), nil))
	if rec["editable"] != true || record(wife.do("GET", "/api/records/"+id("b"), nil))["editable"] != false {
		t.Fatal("record editable flags")
	}
	note := func(c *client, rid string) resp {
		rv := record(c.do("GET", "/api/records/"+rid, nil))["revs"].(map[string]any)["note"]
		return c.do("PATCH", "/api/records/"+rid, map[string]any{"if_rev": rv, "note": "от гостя"})
	}
	expect(t, note(wife, id("a")), http.StatusOK, "edit in edit group")
	expect(t, note(wife, id("b")), http.StatusForbidden, "edit in read-only group")
	expect(t, wife.do("PATCH", "/api/records/"+id("h"), map[string]any{"if_rev": 0, "note": "x"}), http.StatusNotFound, "edit invisible")
	expect(t, wife.do("DELETE", "/api/records/"+id("a"), nil), http.StatusForbidden, "guest delete")
	expect(t, wife.do("POST", "/api/records/"+id("rel")+"/participants", map[string]any{"if_rev": 0, "entity": id("a"), "role": "friend"}),
		http.StatusForbidden, "participant into read-only container")

	// создание: без групп — в технической группе, гостю она не видна, запись правится
	c := batch(t, wife, dash, map[string]any{"records": []any{map[string]any{"ref": "n", "entity": "kin.core.person",
		"fields": map[string]any{"name": []any{map[string]any{"value": "Новый"}}}}}}, http.StatusCreated)
	nid := created(t, c, "n")
	mine := record(wife.do("GET", "/api/records/"+nid, nil))
	if mine["editable"] != true || len(mine["groups"].([]any)) != 0 {
		t.Fatalf("guest's new record: editable %v groups %v", mine["editable"], mine["groups"])
	}
	expect(t, note(wife, nid), http.StatusOK, "edit own new record")
	ownerView := record(owner.do("GET", "/api/records/"+nid, nil))
	if len(ownerView["groups"].([]any)) != 1 {
		t.Fatalf("owner sees technical label %v", ownerView["groups"])
	}
	tech := ownerView["groups"].([]any)[0].(string)
	if g := groupsOf(t, owner, dash)[tech]; g == nil || g["name"] != "Созданные mama" {
		t.Fatalf("technical group %v", g)
	}
	if groupsOf(t, wife, dash)[tech] != nil {
		t.Fatal("technical group shown to guest")
	}
	// второе создание — та же техническая группа
	c2 := batch(t, wife, dash, map[string]any{"records": []any{map[string]any{"ref": "m", "entity": "kin.core.person", "groups": []any{fam}}}}, http.StatusCreated)
	if gs := record(owner.do("GET", "/api/records/"+created(t, c2, "m"), nil))["groups"].([]any); len(gs) != 2 || !slices.Contains(gs, any(tech)) {
		t.Fatalf("second creation labels %v", gs)
	}
	batch(t, wife, dash, map[string]any{"records": []any{map[string]any{"ref": "x", "entity": "kin.core.person", "groups": []any{ro}}}}, http.StatusBadRequest)
	batch(t, wife, dash, map[string]any{"records": []any{map[string]any{"ref": "x", "entity": "kin.core.person", "groups": []any{priv}}}}, http.StatusBadRequest)

	// метки: гость меняет только свои группы, невидимая метка сохраняется
	ownA := record(owner.do("GET", "/api/records/"+id("a"), nil))
	expect(t, owner.do("PUT", "/api/records/"+id("a")+"/groups", map[string]any{"if_rev": ownA["revs"].(map[string]any)["groups"], "groups": []string{fam, priv}}),
		http.StatusOK, "owner labels")
	wa := record(wife.do("GET", "/api/records/"+id("a"), nil))
	expect(t, wife.do("PUT", "/api/records/"+id("a")+"/groups", map[string]any{"if_rev": wa["revs"].(map[string]any)["groups"], "groups": []string{fam}}),
		http.StatusOK, "guest sets own labels")
	if gs := record(owner.do("GET", "/api/records/"+id("a"), nil))["groups"].([]any); len(gs) != 2 {
		t.Fatalf("hidden label lost %v", gs)
	}
	// стоящая метка группы «только просмотр» не мешает гостю менять свои метки
	ownA = record(owner.do("GET", "/api/records/"+id("a"), nil))
	expect(t, owner.do("PUT", "/api/records/"+id("a")+"/groups", map[string]any{"if_rev": ownA["revs"].(map[string]any)["groups"], "groups": []string{fam, priv, ro}}),
		http.StatusOK, "owner adds read-only label")
	wa = record(wife.do("GET", "/api/records/"+id("a"), nil))
	expect(t, wife.do("PUT", "/api/records/"+id("a")+"/groups", map[string]any{"if_rev": wa["revs"].(map[string]any)["groups"], "groups": []string{fam, ro}}),
		http.StatusOK, "guest saves labels with a read-only one")
	if gs := record(owner.do("GET", "/api/records/"+id("a"), nil))["groups"].([]any); len(gs) != 3 {
		t.Fatalf("labels after guest edit %v", gs)
	}

	// журнал гостя — без его технической группы, выдач и меток невидимых групп (F-149)
	for _, o := range auditPageOf(t, wife, dash, url.Values{}).Operations {
		for _, en := range o.Entries {
			if en.Object == "grp" || en.Object == "grp_grant" || (en.Object == "entity_group" && en.After["group_id"] == tech) {
				t.Fatalf("guest audit shows %s %v", en.Object, en.After)
			}
		}
	}
	// право правки на техническую группу — право создавать (F-149): владелец
	// понизил его — гость не создаёт и своё созданное только смотрит
	grantTech := func(edit bool) {
		t.Helper()
		expect(t, owner.do("PUT", "/api/dashboards/"+dash+"/grants", map[string]any{"nick": "mama", "group_id": tech, "can_edit": edit}), http.StatusOK, "tech grant")
	}
	grantTech(false)
	if ds := wife.doList("/api/dashboards"); ds[1]["can_edit"] != false {
		t.Fatalf("can_edit after tech downgrade %v", ds)
	}
	batch(t, wife, dash, map[string]any{"records": []any{map[string]any{"ref": "k", "entity": "kin.core.person"}}}, http.StatusForbidden)
	expect(t, note(wife, nid), http.StatusForbidden, "own record after tech downgrade")
	uploadPNG(t, wife, dash, 9) // файлы к редактируемым записям — по-прежнему
	grantTech(true)
	batch(t, wife, dash, map[string]any{"records": []any{map[string]any{"ref": "k", "entity": "kin.core.person"}}}, http.StatusCreated)
	// техническую группу не удалить, пока гость в дашборде
	expect(t, owner.do("DELETE", "/api/dashboards/"+dash+"/groups/"+tech, nil), http.StatusBadRequest, "delete tech group")
	// отозванное владельцем право на неё не возвращается другими выдачами
	var wifeID string
	for _, m := range owner.doList("/api/dashboards/" + dash + "/members") {
		if m["nick"] == "mama" {
			wifeID = m["account_id"].(string)
		}
	}
	expect(t, owner.do("DELETE", "/api/dashboards/"+dash+"/grants/"+tech+"/"+wifeID, nil), http.StatusOK, "revoke tech")
	share(t, owner, wife, dash, "mama", fam, true)
	batch(t, wife, dash, map[string]any{"records": []any{map[string]any{"ref": "k", "entity": "kin.core.person"}}}, http.StatusForbidden)
	// вложена в группу с правкой, но своё право — просмотр: не создаёт
	expect(t, owner.do("PATCH", "/api/dashboards/"+dash+"/groups/"+tech, map[string]any{"parents": []string{fam}}), http.StatusOK, "nest tech")
	grantTech(false)
	if ds := wife.doList("/api/dashboards"); ds[1]["can_edit"] != false {
		t.Fatalf("can_edit with nested tech group %v", ds)
	}
	batch(t, wife, dash, map[string]any{"records": []any{map[string]any{"ref": "k", "entity": "kin.core.person"}}}, http.StatusForbidden)
	expect(t, owner.do("PATCH", "/api/dashboards/"+dash+"/groups/"+tech, map[string]any{"parents": []string{}}), http.StatusOK, "unnest tech")
	grantTech(true)

	// привязка даты к скрытому событию: ни в журнале гостя, ни в ответе 409 (F-05)
	hidden := batch(t, owner, dash, map[string]any{"records": []any{
		map[string]any{"ref": "w", "template": "kin.core.wedding", "groups": []any{priv}},
		map[string]any{"ref": "p1", "entity": "kin.core.person", "groups": []any{priv}},
		map[string]any{"ref": "p2", "entity": "kin.core.person", "groups": []any{priv}},
	}, "participants": []any{
		map[string]any{"container": "w", "entity": "p1", "role": "partner"},
		map[string]any{"container": "w", "entity": "p2", "role": "partner"},
	}}, http.StatusCreated)
	wid := created(t, hidden, "w")
	ownA = record(owner.do("GET", "/api/records/"+id("a"), nil))
	expect(t, owner.do("PATCH", "/api/records/"+id("a"), map[string]any{"if_rev": ownA["revs"].(map[string]any)["start"],
		"start": map[string]any{"value": nil, "link": wid}}), http.StatusOK, "hidden start link")
	expect(t, note(wife, id("a")), http.StatusOK, "guest note after hidden link")
	for _, o := range auditPageOf(t, wife, dash, url.Values{"entity": {id("a")}}).Operations {
		for _, en := range o.Entries {
			if en.Object == "entity" && en.After["start_link"] != nil {
				t.Fatalf("hidden link in guest audit %v", en.After)
			}
		}
	}
	// привязка значения к скрытому событию: правка значения гостем — в его журнале без неё
	ownA = record(owner.do("GET", "/api/records/"+id("a"), nil))
	sv := owner.do("POST", "/api/records/"+id("a")+"/fields/surname/values", map[string]any{"if_rev": fieldRev(ownA, "surname"),
		"value": "Петрова", "links": []any{map[string]any{"target": wid, "anchor": "start"}}})
	expect(t, sv, http.StatusOK, "owner value with hidden link")
	vid := values(record(sv), "surname")[0]["id"].(string)
	wa = record(wife.do("GET", "/api/records/"+id("a"), nil))
	expect(t, wife.do("PATCH", "/api/records/"+id("a")+"/fields/surname/values/"+vid, map[string]any{"if_rev": fieldRev(wa, "surname"), "note": "от гостя"}),
		http.StatusOK, "guest patches value")
	for _, o := range auditPageOf(t, wife, dash, url.Values{"entity": {id("a")}}).Operations {
		for _, en := range o.Entries {
			if en.Object == "entry_link" || strings.Contains(fmt.Sprint(en.After), wid) {
				t.Fatalf("hidden value link in guest audit %s %v", en.Object, en.After)
			}
		}
	}
	conflict := wife.do("PATCH", "/api/records/"+id("a"), map[string]any{"if_rev": 0, "start": map[string]any{"value": map[string]any{"y": 1990}}})
	expect(t, conflict, http.StatusConflict, "stale start")
	// у даты только скрытая привязка — для гостя значения нет вовсе
	if cur := conflict.body["current"].(map[string]any)["value"]; cur != nil {
		t.Fatalf("hidden link in 409 %v", cur)
	}

	// гость только с просмотром не создаёт и файлов не загружает: техническая
	// группа у него есть, но с просмотром
	son := guest(t, e, owner, "son", "sonny")
	share(t, owner, son, dash, "sonny", ro, false)
	if ds := son.doList("/api/dashboards"); ds[1]["can_edit"] != false {
		t.Fatalf("read-only guest can_edit %v", ds)
	}
	batch(t, son, dash, map[string]any{"records": []any{map[string]any{"ref": "p", "entity": "kin.core.person"}}}, http.StatusForbidden)
	if st, _ := son.upload(dash, []byte("\x89PNG")); st != http.StatusNotFound {
		t.Fatalf("read-only guest upload %d", st)
	}
	if g := groupsOf(t, owner, dash); !slices.ContainsFunc(slices.Collect(maps.Values(g)), func(x map[string]any) bool { return x["name"] == "Созданные sonny" }) {
		t.Fatal("technical group of read-only guest")
	}
}

// F-148: связи и файлы — по тем же правам, что записи: связь «только просмотр»
// гость не правит, невидимую — как несуществующую; невидимый человек
// участником не ставится — ответ как на несуществующего (F-07); файл
// прикрепляется только к записи с правкой.
func TestGuestRelationsAndFiles(t *testing.T) {
	e := newEnv(t)
	owner := e.setupOwner()
	dash := dashboardOf(t, owner)
	fam, ro, priv := group(t, owner, dash, "Семья"), group(t, owner, dash, "Соседи"), group(t, owner, dash, "Личное")
	r := batch(t, owner, dash, map[string]any{"records": []any{
		map[string]any{"ref": "a", "entity": "kin.core.person", "groups": []any{fam}},
		map[string]any{"ref": "a2", "entity": "kin.core.person", "groups": []any{fam}},
		map[string]any{"ref": "a3", "entity": "kin.core.person", "groups": []any{fam}},
		map[string]any{"ref": "b", "entity": "kin.core.person", "groups": []any{ro}},
		map[string]any{"ref": "c", "entity": "kin.core.person", "groups": []any{ro}},
		map[string]any{"ref": "h", "entity": "kin.core.person", "groups": []any{priv}},
		map[string]any{"ref": "h2", "entity": "kin.core.person", "groups": []any{priv}},
		map[string]any{"ref": "frel", "template": "kin.core.friend", "groups": []any{fam}},
		map[string]any{"ref": "com", "template": "kin.core.commune", "groups": []any{fam}},
		map[string]any{"ref": "rel", "template": "kin.core.friend", "groups": []any{ro}},
		map[string]any{"ref": "hrel", "template": "kin.core.friend", "groups": []any{priv}},
	}, "participants": []any{
		map[string]any{"container": "frel", "entity": "a", "role": "friend"},
		map[string]any{"container": "frel", "entity": "a2", "role": "friend"},
		map[string]any{"container": "com", "entity": "a", "role": "member"},
		map[string]any{"container": "rel", "entity": "b", "role": "friend"},
		map[string]any{"container": "rel", "entity": "c", "role": "friend"},
		map[string]any{"container": "hrel", "entity": "h", "role": "friend"},
		map[string]any{"container": "hrel", "entity": "h2", "role": "friend"},
	}}, http.StatusCreated)
	id := func(ref string) string { return created(t, r, ref) }
	wife := guest(t, e, owner, "wife", "mama")
	share(t, owner, wife, dash, "mama", fam, true)
	share(t, owner, wife, dash, "mama", ro, false)

	note := func(rid string) resp {
		rv := record(wife.do("GET", "/api/records/"+rid, nil))["revs"].(map[string]any)["note"]
		return wife.do("PATCH", "/api/records/"+rid, map[string]any{"if_rev": rv, "note": "от гостя"})
	}
	expect(t, note(id("frel")), http.StatusOK, "edit editable relation")
	expect(t, note(id("rel")), http.StatusForbidden, "edit read-only relation")
	expect(t, wife.do("PATCH", "/api/records/"+id("hrel"), map[string]any{"if_rev": 0, "note": "x"}), http.StatusNotFound, "edit invisible relation")
	expect(t, wife.do("POST", "/api/records/"+id("hrel")+"/participants", map[string]any{"if_rev": 0, "entity": id("a"), "role": "friend"}),
		http.StatusNotFound, "participant into invisible relation")
	rels := record(wife.do("GET", "/api/records/"+id("rel"), nil))
	if ps := rels["participants"].([]any); len(ps) != 2 {
		t.Fatalf("read-only relation participants %v", ps)
	}
	pid := rels["participants"].([]any)[0].(map[string]any)["id"].(string)
	expect(t, wife.do("DELETE", "/api/records/"+id("rel")+"/participants/"+pid, nil), http.StatusForbidden, "remove participant of read-only relation")

	// невидимый участник — как несуществующий (в общине мест без предела)
	add := func(entity string) resp {
		rv := record(wife.do("GET", "/api/records/"+id("com"), nil))["revs"].(map[string]any)["participants:member"]
		return wife.do("POST", "/api/records/"+id("com")+"/participants", map[string]any{"if_rev": rv, "entity": entity, "role": "member"})
	}
	hidden, missing := add(id("h")), add(uuid.NewString())
	if hidden.status < 400 || hidden.status != missing.status || hidden.body["message"] != missing.body["message"] {
		t.Fatalf("invisible participant %d %v, missing %d %v", hidden.status, hidden.body, missing.status, missing.body)
	}
	expect(t, add(id("a3")), http.StatusOK, "visible participant")

	// файл: загружает гость с правом создавать; прикрепить — только к записи с правкой
	sha := uploadPNG(t, wife, dash, 7)
	photo := func(rid string) resp {
		return wife.do("POST", "/api/records/"+rid+"/fields/photo/values", map[string]any{"if_rev": fieldRev(record(wife.do("GET", "/api/records/"+rid, nil)), "photo"), "value": sha})
	}
	expect(t, photo(id("b")), http.StatusForbidden, "photo on read-only record")
	expect(t, wife.do("POST", "/api/records/"+id("h")+"/fields/photo/values", map[string]any{"if_rev": 0, "value": sha}), http.StatusNotFound, "photo on invisible record")
	expect(t, photo(id("a")), http.StatusOK, "photo on editable record")
}

// Q-50: одиночное поле, занятое ссылкой на невидимую гостю запись, гостю —
// «Задано» (hidden) без значения и без правки; владелец видит значение.
func TestGuestHiddenSingleRef(t *testing.T) {
	e := newEnv(t)
	owner := e.setupOwner()
	dash := dashboardOf(t, owner)
	fam, priv := group(t, owner, dash, "Семья"), group(t, owner, dash, "Личное")
	anna := person(t, owner, dash, map[string]any{"groups": []any{fam}})
	ivan := person(t, owner, dash, map[string]any{"groups": []any{fam}})
	petya := person(t, owner, dash, map[string]any{"groups": []any{priv}})
	expect(t, owner.do("PATCH", "/api/records/"+anna, map[string]any{"if_rev": 0, "custom_fields": []any{
		map[string]any{"key": "godfather", "type": "ref", "name": "Крёстный"},
		map[string]any{"key": "friends", "type": "ref", "name": "Друзья", "multiple": true},
	}}), http.StatusOK, "custom fields")
	addValue(t, owner, anna, "custom.godfather", map[string]any{"if_rev": 0, "value": petya})
	addValue(t, owner, anna, "custom.friends", map[string]any{"if_rev": 0, "value": petya})
	wife := guest(t, e, owner, "wife", "mama")
	share(t, owner, wife, dash, "mama", fam, true)

	rec := record(wife.do("GET", "/api/records/"+anna, nil))
	f := rec["fields"].(map[string]any)["custom.godfather"].(map[string]any)
	if f["hidden"] != true || len(f["values"].([]any)) != 0 {
		t.Fatalf("hidden single ref %v", f)
	}
	// у списка значений скрытое просто не показывается — добавлять можно
	if fr := rec["fields"].(map[string]any)["custom.friends"].(map[string]any); fr["hidden"] != nil || len(fr["values"].([]any)) != 0 {
		t.Fatalf("hidden multiple ref %v", fr)
	}
	expect(t, wife.do("POST", "/api/records/"+anna+"/fields/custom.godfather/values", map[string]any{"if_rev": fieldRev(rec, "custom.godfather"), "value": ivan}),
		http.StatusForbidden, "guest fills hidden single field")
	expect(t, wife.do("POST", "/api/records/"+anna+"/fields/custom.friends/values", map[string]any{"if_rev": fieldRev(rec, "custom.friends"), "value": ivan}),
		http.StatusOK, "guest adds to multiple field")
	own := record(owner.do("GET", "/api/records/"+anna, nil))
	if v := values(own, "custom.godfather"); len(v) != 1 || v[0]["value"] != petya {
		t.Fatalf("owner value %v", v)
	}
	if own["fields"].(map[string]any)["custom.godfather"].(map[string]any)["hidden"] != nil {
		t.Fatal("owner sees hidden flag")
	}
}

// F-149: смена ника гостя меняет ник в названии его технической группы —
// и в переименованном владельцем названии, целым словом.
func TestGuestGroupNick(t *testing.T) {
	e := newEnv(t)
	owner := e.setupOwner()
	dash := dashboardOf(t, owner)
	fam := group(t, owner, dash, "Семья")
	wife := guest(t, e, owner, "wife", "mama")
	share(t, owner, wife, dash, "mama", fam, true)
	techOf := func() (string, string) {
		t.Helper()
		for id, g := range groupsOf(t, owner, dash) {
			if id != fam {
				return id, g["name"].(string)
			}
		}
		t.Fatal("no technical group")
		return "", ""
	}
	tech, name := techOf()
	if name != "Созданные mama" {
		t.Fatalf("technical group name %q", name)
	}
	expect(t, wife.do("PUT", "/api/me/nick", map[string]any{"nick": "mom"}), http.StatusOK, "nick")
	if _, name := techOf(); name != "Созданные mom" {
		t.Fatalf("after nick change %q", name)
	}
	expect(t, owner.do("PATCH", "/api/dashboards/"+dash+"/groups/"+tech, map[string]any{"name": "Записи mom (momma)"}), http.StatusOK, "owner renames")
	expect(t, wife.do("PUT", "/api/me/nick", map[string]any{"nick": "mother"}), http.StatusOK, "nick again")
	if _, name := techOf(); name != "Записи mother (momma)" {
		t.Fatalf("after owner rename and nick change %q", name)
	}
	// без ника в названии — логин
	expect(t, wife.do("PUT", "/api/me/nick", map[string]any{"nick": ""}), http.StatusOK, "clear nick")
	if _, name := techOf(); name != "Записи wife (momma)" {
		t.Fatalf("after nick cleared %q", name)
	}
	// гость без ника: в списке — логин, права меняются по аккаунту
	var wifeID string
	for _, m := range owner.doList("/api/dashboards/" + dash + "/members") {
		if m["login"] == "wife" && m["nick"] == nil {
			wifeID = m["account_id"].(string)
		}
	}
	if wifeID == "" {
		t.Fatal("nickless member without login")
	}
	other := group(t, owner, dash, "Друзья")
	for _, g := range []string{fam, other, tech} {
		expect(t, owner.do("PUT", "/api/dashboards/"+dash+"/grants", map[string]any{"account_id": wifeID, "group_id": g, "can_edit": false}),
			http.StatusOK, "grant by account")
	}
	for _, m := range owner.doList("/api/dashboards/" + dash + "/members") {
		if m["account_id"] == wifeID {
			if gs := m["grants"].([]any); len(gs) != 3 || slices.ContainsFunc(gs, func(g any) bool { return g.(map[string]any)["can_edit"] == true }) {
				t.Fatalf("grants by account %v", gs)
			}
		}
	}
	// по id — только гости этого дашборда
	guest(t, e, owner, "stranger", "")
	var strangerID string
	for _, a := range owner.doList("/api/admin/accounts") {
		if a["login"] == "stranger" {
			strangerID = a["id"].(string)
		}
	}
	expect(t, owner.do("PUT", "/api/dashboards/"+dash+"/grants", map[string]any{"account_id": strangerID, "group_id": fam}),
		http.StatusBadRequest, "grant by account to non-member")
	found := false
	for _, o := range auditPageOf(t, owner, dash, url.Values{}).Operations {
		for _, en := range o.Entries {
			found = found || (en.Object == "grp" && en.After["name"] == "Записи mother (momma)")
		}
	}
	if !found {
		t.Fatal("rename not in audit")
	}
}
