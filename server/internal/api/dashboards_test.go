package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"kintion/server/internal/dashboards"
	"kintion/server/internal/db"
	"kintion/server/internal/db/sqlc"
	"kintion/server/internal/packs"
	"kintion/server/internal/version"
)

func auditOf(t *testing.T, e *env, op uuid.UUID) []sqlc.AuditLog {
	t.Helper()
	rows, err := sqlc.New(e.pool).ListAuditByOperation(context.Background(), op)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// operationOf — operation_id записей журнала с request_id запроса.
func operationOf(t *testing.T, e *env, requestID string) uuid.UUID {
	t.Helper()
	var op uuid.UUID
	if err := e.pool.QueryRow(context.Background(),
		"SELECT DISTINCT operation_id FROM audit_log WHERE request_id = $1", requestID).Scan(&op); err != nil {
		t.Fatalf("operation of request %s: %v", requestID, err)
	}
	return op
}

func jsonField(t *testing.T, raw []byte, key string) any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m[key]
}

func TestSetupCreatesFirstDashboard(t *testing.T) {
	e := newEnv(t)
	c := e.client()
	r := c.do("POST", "/api/setup", map[string]string{"token": e.token, "login": "owner", "password": "secret-pass"})
	expect(t, r, http.StatusCreated, "setup")
	list := c.doList("/api/dashboards")
	if len(list) != 1 || list[0]["name"] != "Мой дашборд" {
		t.Fatalf("dashboards %v", list)
	}
	rows := auditOf(t, e, operationOf(t, e, r.header.Get("X-Request-Id")))
	// аккаунт + дашборд + шаблоны обоих паков — одна операция
	if len(rows) != 2+len(e.reg.Templates()) || rows[0].Object != "account" || rows[1].Object != "dashboard" {
		t.Fatalf("setup audit: %d rows", len(rows))
	}
}

func TestDashboardAudit(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	r := c.do("POST", "/api/dashboards", map[string]string{"name": "  Работа "})
	expect(t, r, http.StatusCreated, "create")
	id, _ := r.body["id"].(string)
	if r.body["name"] != "Работа" {
		t.Fatalf("created %v", r.body)
	}
	reqID := r.header.Get("X-Request-Id")
	rows := auditOf(t, e, operationOf(t, e, reqID))
	if len(rows) != 1+len(e.reg.Templates()) {
		t.Fatalf("create audit rows %d", len(rows))
	}
	first := rows[0]
	if first.Object != "dashboard" || first.Op != "create" || first.Before != nil ||
		jsonField(t, first.After, "name") != "Работа" || first.ObjectID.String() != id ||
		first.AppVersion != version.Version || first.SchemaVersion != db.SchemaVersion() ||
		first.RequestID != reqID || first.Source != "ui" || first.ActorID == nil || first.DashboardID.String() != id {
		t.Fatalf("create audit %+v", first)
	}
	for _, row := range rows[1:] {
		if row.Object != "dashboard_template" || row.OperationID != first.OperationID {
			t.Fatalf("template audit %+v", row)
		}
	}

	expect(t, c.do("PATCH", "/api/dashboards/"+id, map[string]string{"name": ""}), http.StatusBadRequest, "empty name")
	r = c.do("PATCH", "/api/dashboards/"+id, map[string]string{"name": "Семья"})
	expect(t, r, http.StatusOK, "rename")
	rows = auditOf(t, e, operationOf(t, e, r.header.Get("X-Request-Id")))
	if len(rows) != 1 || rows[0].Op != "update" || jsonField(t, rows[0].Before, "name") != "Работа" ||
		jsonField(t, rows[0].After, "name") != "Семья" || rows[0].RequestID != r.header.Get("X-Request-Id") {
		t.Fatalf("rename audit %+v", rows)
	}
	if list := c.doList("/api/dashboards"); len(list) != 2 {
		t.Fatalf("dashboards %v", list)
	}
}

func TestForeignDashboardNotFound(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	ctx := context.Background()
	other, foreign := uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7())
	if _, err := e.pool.Exec(ctx, "INSERT INTO account (id, login, password_hash) VALUES ($1, 'other', 'x')", other); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(ctx, "INSERT INTO dashboard (id, owner_id, name) VALUES ($1, $2, 'чужой')", foreign, other); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{foreign.String(), uuid.Must(uuid.NewV7()).String(), "not-a-uuid"} {
		expect(t, c.do("GET", "/api/dashboards/"+id+"/schema", nil), http.StatusNotFound, "schema "+id)
		expect(t, c.do("PATCH", "/api/dashboards/"+id, map[string]string{"name": "x"}), http.StatusNotFound, "rename "+id)
	}
	for _, d := range c.doList("/api/dashboards") {
		if d["id"] == foreign.String() {
			t.Fatal("foreign dashboard listed")
		}
	}
}

type schemaResp struct {
	Language  string            `json:"language"`
	Entities  []json.RawMessage `json:"entities"`
	Templates []struct {
		ID         string          `json:"id"`
		Key        string          `json:"key"`
		Pack       string          `json:"pack"`
		Default    bool            `json:"default"`
		Definition json.RawMessage `json:"definition"`
	} `json:"templates"`
}

func (c *client) schema(id string) schemaResp {
	c.e.t.Helper()
	var s schemaResp
	c.getJSON("/api/dashboards/"+id+"/schema", &s)
	return s
}

func countTemplates(t *testing.T, e *env, dashboard string, where string) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM dashboard_template WHERE dashboard_id = $1 AND "+where, dashboard).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSchemaAndInactiveTemplates(t *testing.T) {
	e := newEnv(t)
	c := e.setupOwner()
	id, _ := c.doList("/api/dashboards")[0]["id"].(string)
	s := c.schema(id)
	if s.Language != "ru" || len(s.Entities) != 4 || len(s.Templates) != 20 {
		t.Fatalf("schema: %d entities, %d templates", len(s.Entities), len(s.Templates))
	}
	defaults := 0
	for _, tm := range s.Templates {
		if tm.Default {
			defaults++
		}
		if jsonField(t, tm.Definition, "id") != tm.Key {
			t.Fatalf("definition of %s", tm.Key)
		}
	}
	if defaults != 3 {
		t.Fatalf("default templates %d", defaults)
	}

	// пак удалён: его шаблоны неактивны и в реестр дашборда не попадают (F-60)
	ctx := context.Background()
	src, _ := packs.Source("", "ru")
	core, _ := packs.Load(src, "ru", []string{"kin.core"})
	if err := dashboards.SyncTemplates(ctx, e.pool, core); err != nil {
		t.Fatal(err)
	}
	if n := countTemplates(t, e, id, "NOT active AND pack_id <> 'kin.core'"); n != 3 {
		t.Fatalf("inactive %d", n)
	}
	if n := countTemplates(t, e, id, "active"); n != 17 {
		t.Fatalf("active %d", n)
	}
	// пак вернули — снова активны
	if err := dashboards.SyncTemplates(ctx, e.pool, e.reg); err != nil {
		t.Fatal(err)
	}
	if n := countTemplates(t, e, id, "active"); n != 20 {
		t.Fatalf("active after pack is back %d", n)
	}
}

// Реестр сервера без пака: шаблоны пака в БД есть, но schema их не отдаёт.
func TestSchemaSkipsTemplatesOfMissingPack(t *testing.T) {
	e := newEnv(t, "kin.core")
	c := e.setupOwner()
	id, _ := c.doList("/api/dashboards")[0]["id"].(string)
	if s := c.schema(id); len(s.Templates) != 17 {
		t.Fatalf("core only: %d", len(s.Templates))
	}
	// пак включили (рестарт с TEO_KIN_PACKS): шаблоны добавляются в существующий дашборд
	src, _ := packs.Source("", "ru")
	both, _ := packs.Load(src, "ru", []string{"kin.christian.orthodox"})
	ctx := context.Background()
	for range 2 { // повторная синхронизация ничего не добавляет
		if err := dashboards.SyncTemplates(ctx, e.pool, both); err != nil {
			t.Fatal(err)
		}
		if n := countTemplates(t, e, id, "true"); n != 20 {
			t.Fatalf("templates after sync: %d", n)
		}
	}
	// этот сервер работает со старым реестром — лишнего не показывает
	if s := c.schema(id); len(s.Templates) != 17 {
		t.Fatalf("schema with registry of core only: %d", len(s.Templates))
	}
}

func TestEffectiveOverrides(t *testing.T) {
	base := json.RawMessage(`{"id":"t","name":"Пак","fields":[]}`)
	got, err := dashboards.Effective(base, json.RawMessage(`{}`))
	if err != nil || string(got) != string(base) {
		t.Fatalf("no overrides: %s %v", got, err)
	}
	got, err = dashboards.Effective(base, json.RawMessage(`{"name":"Своё"}`))
	if err != nil || jsonField(t, got, "name") != "Своё" || jsonField(t, got, "id") != "t" {
		t.Fatalf("override: %s %v", got, err)
	}
}
