//go:build postgres

package parcelcrm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestTasksOwnershipRetriesAndStub(t *testing.T) {
	s, a, b, parcel := fixture(t)
	ctx := context.Background()
	if _, e := s.favorite(ctx, a, parcel, nil); e != nil {
		t.Fatal(e)
	}
	if _, e := s.favorite(ctx, b, parcel, nil); e != nil {
		t.Fatal(e)
	}
	id := "cbac2995-30ae-4e20-89b0-1a1035c17405"
	due := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339)
	t1, e := s.createTask(ctx, a, parcel, id, "Private text", due, "UTC")
	if e != nil {
		t.Fatal(e)
	}
	if t1.ReminderState != "queued" || t1.ScheduleVersion != 1 {
		t.Fatalf("created %+v", t1)
	}
	retry, e := s.createTask(ctx, a, parcel, id, "Private text", due, "UTC")
	if e != nil || retry.Revision != t1.Revision {
		t.Fatalf("retry %+v %v", retry, e)
	}
	if _, e = s.createTask(ctx, a, parcel, id, "Different text", due, "UTC"); !errors.Is(e, errConflict) {
		t.Fatalf("changed retry %v", e)
	}
	if _, e = s.task(ctx, b, parcel, id); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("cross account read %v", e)
	}
	if _, e = s.changeTask(ctx, b, parcel, id, "complete", "", "", "", 1); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("cross account write %v", e)
	}
	if _, e = s.changeTask(ctx, a, parcel, id, "stub", "", "", "", t1.Revision); !errors.Is(e, errInvalid) {
		t.Fatalf("premature stub %v", e)
	}
	updated, e := s.changeTask(ctx, a, parcel, id, "reschedule", "Private text", time.Now().Add(time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339), "UTC", t1.Revision)
	if e != nil {
		t.Fatal(e)
	}
	if updated.ScheduleVersion != 2 {
		t.Fatalf("schedule version %+v", updated)
	}
	if _, e = s.changeTask(ctx, a, parcel, id, "stub", "", "", "", t1.Revision); !errors.Is(e, errConflict) {
		t.Fatalf("stale stub %v", e)
	}
	var oldState string
	if e = s.DB.QueryRow(ctx, `SELECT state FROM user_task_reminders WHERE user_id=$1 AND task_id=$2 AND version=1`, string(a), id).Scan(&oldState); e != nil || oldState != "canceled" {
		t.Fatalf("old intent %q %v", oldState, e)
	}
	card, e := s.card(ctx, a, parcel)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.mutate(ctx, a, parcel, "exclude", "", "not for now", card.Revision); e != nil {
		t.Fatal(e)
	}
	if _, e = s.changeTask(ctx, a, parcel, id, "reschedule", "Private text", due, "UTC", updated.Revision); !errors.Is(e, errConflict) {
		t.Fatalf("excluded schedule %v", e)
	}
	var canceled string
	if e = s.DB.QueryRow(ctx, `SELECT state FROM user_task_reminders WHERE user_id=$1 AND task_id=$2 AND version=2`, string(a), id).Scan(&canceled); e != nil || canceled != "canceled" {
		t.Fatalf("exclusion %q %v", canceled, e)
	}
	card, e = s.card(ctx, a, parcel)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.mutate(ctx, a, parcel, "restore", "", "", card.Revision); e != nil {
		t.Fatal(e)
	}
	if e = s.DB.QueryRow(ctx, `SELECT state FROM user_task_reminders WHERE user_id=$1 AND task_id=$2 AND version=2`, string(a), id).Scan(&canceled); e != nil || canceled != "canceled" {
		t.Fatal("restore reactivated intent")
	}
	again, e := s.changeTask(ctx, a, parcel, id, "reschedule", "Again", due, "UTC", updated.Revision)
	if e != nil {
		t.Fatal(e)
	}
	if again.ScheduleVersion != 3 {
		t.Fatalf("restored schedule %+v", again)
	}
	// Simulate passage of time using the disposable fixture only; no provider or external worker is invoked.
	_, e = s.DB.Exec(ctx, `UPDATE user_parcel_tasks SET due_at=now()-interval '5 minutes' WHERE user_id=$1 AND id=$2`, string(a), id)
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.DB.Exec(ctx, `UPDATE user_task_reminders SET due_at=now()-interval '5 minutes' WHERE user_id=$1 AND task_id=$2 AND version=3`, string(a), id)
	if e != nil {
		t.Fatal(e)
	}
	stub, e := s.changeTask(ctx, a, parcel, id, "stub", "", "", "", again.Revision)
	if e != nil || stub.ReminderState != "stubbed" || stub.Status != "open" {
		t.Fatalf("stub %+v %v", stub, e)
	}
	if _, e = s.changeTask(ctx, a, parcel, id, "stub", "", "", "", again.Revision); !errors.Is(e, errConflict) {
		t.Fatalf("duplicate stub %v", e)
	}
	completed, e := s.changeTask(ctx, a, parcel, id, "complete", "", "", "", stub.Revision)
	if e != nil || completed.Status != "completed" || completed.ReminderState != "stubbed" {
		t.Fatalf("complete %+v %v", completed, e)
	}
	page, e := s.tasks(ctx, a, "", "", 20)
	if e != nil || len(page.Items) != 0 {
		t.Fatalf("completed still open %+v %v", page, e)
	}
}
func TestMissedRemovalAndDatabaseOwnership(t *testing.T) {
	s, a, b, parcel := fixture(t)
	ctx := context.Background()
	card, e := s.favorite(ctx, a, parcel, nil)
	if e != nil {
		t.Fatal(e)
	}
	id := "19e47c26-82b5-4a32-bbf0-3dbf5be08a88"
	due := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339)
	task, e := s.createTask(ctx, a, parcel, id, "Not sent", due, "UTC")
	if e != nil {
		t.Fatal(e)
	}
	if page, err := s.tasks(ctx, b, parcel, "", 20); err != nil || len(page.Items) != 0 {
		t.Fatalf("foreign list %+v %v", page, err)
	}
	if _, e = s.DB.Exec(ctx, `INSERT INTO user_task_reminders(user_id,parcel_id,task_id,version,due_at,state) VALUES($1,$2,$3,9,now(),'queued')`, string(b), parcel, id); e == nil {
		t.Fatal("foreign reminder FK accepted")
	}
	if _, e = s.DB.Exec(ctx, `UPDATE user_parcel_tasks SET due_at=now()-interval '2 hours' WHERE user_id=$1 AND id=$2`, string(a), id); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE user_task_reminders SET due_at=now()-interval '2 hours' WHERE user_id=$1 AND task_id=$2`, string(a), id); e != nil {
		t.Fatal(e)
	}
	missed, e := s.changeTask(ctx, a, parcel, id, "stub", "", "", "", task.Revision)
	if e != nil || missed.ReminderState != "missed" || missed.Status != "open" {
		t.Fatalf("missed %+v %v", missed, e)
	}
	if _, e = s.changeTask(ctx, a, parcel, id, "stub", "", "", "", missed.Revision); !errors.Is(e, errConflict) {
		t.Fatalf("repeat missed %v", e)
	}
	task, e = s.createTask(ctx, a, parcel, "34a172cc-9d57-4888-a82c-62b073a94ab5", "Pending", due, "UTC")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.mutate(ctx, a, parcel, "remove", "", "", card.Revision); e != nil {
		t.Fatal(e)
	}
	var state string
	if e = s.DB.QueryRow(ctx, `SELECT state FROM user_task_reminders WHERE user_id=$1 AND task_id=$2 AND version=1`, string(a), task.ID).Scan(&state); e != nil || state != "queued" {
		t.Fatalf("removal %s %v", state, e)
	}
	if _, e = s.createTask(ctx, a, parcel, "5030b30d-14cc-4e3a-94de-01f061d67ed8", "no", due, "UTC"); e != nil {
		t.Fatalf("inactive create %v", e)
	}
	if _, e = s.favorite(ctx, a, parcel, &[]int64{card.Revision + 1}[0]); e != nil {
		t.Fatal(e)
	}
	if e = s.DB.QueryRow(ctx, `SELECT state FROM user_task_reminders WHERE user_id=$1 AND task_id=$2 AND version=1`, string(a), task.ID).Scan(&state); e != nil || state != "queued" {
		t.Fatal("restored favorite changed reminder")
	}
	if _, e = s.DB.Exec(ctx, `UPDATE parcels SET parcel_number='REFRESHED' WHERE id=$1`, parcel); e != nil {
		t.Fatal(e)
	}
	if refreshed, err := s.task(ctx, a, parcel, task.ID); err != nil || refreshed.ParcelNumber == nil || *refreshed.ParcelNumber != "REFRESHED" {
		t.Fatalf("source refresh lost task %+v %v", refreshed, err)
	}
	if _, e = s.DB.Exec(ctx, `DELETE FROM parcels WHERE id=$1`, parcel); e == nil {
		t.Fatal("source delete bypassed private task FK")
	}
}

func TestTaskHTTPBoundary(t *testing.T) {
	s, a, b, parcel := fixture(t)
	ctx := context.Background()
	if _, e := s.favorite(ctx, a, parcel, nil); e != nil {
		t.Fatal(e)
	}
	id := "644ea46d-e02a-4395-baff-f35d54d2813b"
	due := time.Now().Add(time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339)
	body := fmt.Sprintf(`{"id":%q,"message":"Only buyer sees this","due_at":%q,"timezone":"UTC"}`, id, due)
	url := "http://app.test/api/me/parcels/" + parcel + "/tasks"
	request := func(method, path, payload, origin string, account AccountID) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		s.Serve(w, req, account, "http://app.test")
		return w
	}
	if w := request("POST", url, body, "http://evil.test", a); w.Code != 403 {
		t.Fatalf("origin %d", w.Code)
	}
	if w := request("POST", url, body+`{"extra":true}`, "http://app.test", a); w.Code != 400 {
		t.Fatalf("extra json %d", w.Code)
	}
	if w := request("POST", url, body, "http://app.test", a); w.Code != 200 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	if w := request("POST", url, body, "http://app.test", a); w.Code != 200 {
		t.Fatalf("retry %d", w.Code)
	}
	if w := request("GET", url+"/"+id, "", "", b); w.Code != 404 {
		t.Fatalf("foreign read %d", w.Code)
	}
	if w := request("GET", url, "", "", b); w.Code != 200 || !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("foreign list %d %s", w.Code, w.Body.String())
	}
	if w := request("POST", url+"/"+id+"/complete", `{"revision":1}`, "http://app.test", b); w.Code != 404 {
		t.Fatalf("foreign write %d", w.Code)
	}
	clear := url + "/clear"
	if w := request("GET", clear, "", "", b); w.Code != 404 {
		t.Fatalf("foreign clear read %d", w.Code)
	}
	if w := request("GET", "http://app.test/api/me/favorites/"+parcel+"/tasks", "", "", a); w.Code != 404 {
		t.Fatalf("legacy task route %d", w.Code)
	}
	count, snapshot, e := s.openTaskSnapshot(ctx, a, parcel)
	if e != nil || count != 1 {
		t.Fatalf("snapshot %d %v", count, e)
	}
	if w := request("GET", clear, "", "", a); w.Code != 200 || !strings.Contains(w.Body.String(), snapshot) {
		t.Fatalf("clear read %d %s", w.Code, w.Body.String())
	}
	payload := fmt.Sprintf(`{"count":1,"snapshot":%q}`, snapshot)
	if w := request("POST", clear, payload, "http://evil.test", a); w.Code != 403 {
		t.Fatalf("clear origin %d", w.Code)
	}
	if w := request("POST", clear, `{"count":1,"snapshot":"bad"}`, "http://app.test", a); w.Code != 400 {
		t.Fatalf("clear token %d", w.Code)
	}
	if w := request("POST", clear, payload, "http://app.test", b); w.Code != 404 {
		t.Fatalf("foreign clear %d", w.Code)
	}
	if w := request("GET", "http://app.test/api/me/crm/tasks?limit=51", "", "", a); w.Code != 400 {
		t.Fatalf("limit %d", w.Code)
	}
}

func TestConcurrentStubClaimsOnce(t *testing.T) {
	s, a, _, parcel := fixture(t)
	ctx := context.Background()
	if _, e := s.favorite(ctx, a, parcel, nil); e != nil {
		t.Fatal(e)
	}
	id := "2cf5e4aa-4dd6-44db-a87d-365815f37080"
	task, e := s.createTask(ctx, a, parcel, id, "Private", time.Now().Add(time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339), "UTC")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE user_parcel_tasks SET due_at=now()-interval '1 minute' WHERE user_id=$1 AND id=$2`, string(a), id); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec(ctx, `UPDATE user_task_reminders SET due_at=now()-interval '1 minute' WHERE user_id=$1 AND task_id=$2`, string(a), id); e != nil {
		t.Fatal(e)
	}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { _, err := s.changeTask(ctx, a, parcel, id, "stub", "", "", "", task.Revision); results <- err }()
	}
	success, conflict := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		switch {
		case err == nil:
			success++
		case errors.Is(err, errConflict):
			conflict++
		default:
			t.Fatalf("claim error %v", err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("claims success=%d conflict=%d", success, conflict)
	}
	var count int
	if e = s.DB.QueryRow(ctx, `SELECT count(*) FROM user_task_reminders WHERE user_id=$1 AND task_id=$2 AND state='stubbed'`, string(a), id).Scan(&count); e != nil || count != 1 {
		t.Fatalf("stubbed rows %d %v", count, e)
	}
}

func TestNonfavoriteTasksAndScopedClear(t *testing.T) {
	s, a, b, parcel := fixture(t)
	ctx := context.Background()
	if page, e := s.tasks(ctx, a, parcel, "", 20); e != nil || len(page.Items) != 0 {
		t.Fatalf("stored parcel without tracking %+v %v", page, e)
	}
	if _, e := s.card(ctx, a, parcel); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("read created tracking %v", e)
	}
	due := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339)
	first, e := s.createTask(ctx, a, parcel, "31218f16-2d23-4e04-983c-4fa859c49ac1", "Buyer", due, "UTC")
	if e != nil {
		t.Fatal(e)
	}
	card, e := s.card(ctx, a, parcel)
	if e != nil || card.Favorite {
		t.Fatalf("task favorited parcel %+v %v", card, e)
	}
	board, e := s.board(ctx, a)
	if e != nil {
		t.Fatal(e)
	}
	if board.States[0].Count != 0 {
		t.Fatal("task appeared on board")
	}
	if _, e = s.task(ctx, b, parcel, first.ID); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("foreign task %v", e)
	}
	foreign, e := s.createTask(ctx, b, parcel, "f48e7d19-d11b-4881-9ac8-901987feec81", "Other buyer", due, "UTC")
	if e != nil {
		t.Fatal(e)
	}
	count, snapshot, e := s.openTaskSnapshot(ctx, a, parcel)
	if e != nil || count != 1 {
		t.Fatalf("snapshot %d %v", count, e)
	}
	if _, e = s.clearParcelTasks(ctx, b, parcel, 1, snapshot); !errors.Is(e, errConflict) {
		t.Fatalf("foreign snapshot %v", e)
	}
	if _, e = s.clearParcelTasks(ctx, a, "00000000-0000-4000-8000-000000000000", 1, snapshot); !errors.Is(e, pgx.ErrNoRows) {
		t.Fatalf("guessed parcel %v", e)
	}
	var otherParcel string
	if e = s.DB.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,parcel_number) VALUES('union','union','union_county_gis',$1,$1) RETURNING id::text`, "task-other-"+parcel).Scan(&otherParcel); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		s.DB.Exec(context.Background(), `DELETE FROM user_parcels WHERE user_id=$1 AND parcel_id=$2`, string(a), otherParcel)
		s.DB.Exec(context.Background(), `DELETE FROM parcels WHERE id=$1`, otherParcel)
	})
	unrelated, e := s.createTask(ctx, a, otherParcel, "9e5af46e-ced8-4d04-9ac9-590b1b241066", "Other parcel", due, "UTC")
	if e != nil {
		t.Fatal(e)
	}
	second, e := s.createTask(ctx, a, parcel, "fe572ada-2bed-4722-9a38-1e6665638505", "Later", due, "UTC")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.clearParcelTasks(ctx, a, parcel, 1, snapshot); !errors.Is(e, errConflict) {
		t.Fatalf("stale count %v", e)
	}
	if _, e = s.changeTask(ctx, a, parcel, second.ID, "complete", "", "", "", second.Revision); e != nil {
		t.Fatal(e)
	}
	count, snapshot, e = s.openTaskSnapshot(ctx, a, parcel)
	if e != nil || count != 1 {
		t.Fatalf("fresh snapshot %d %v", count, e)
	}
	if _, e = s.clearParcelTasks(ctx, a, parcel, count, snapshot); e != nil {
		t.Fatal(e)
	}
	if _, e = s.changeTask(ctx, a, parcel, first.ID, "stub", "", "", "", first.Revision); !errors.Is(e, errConflict) {
		t.Fatalf("cleared version simulated %v", e)
	}
	if other, e := s.task(ctx, b, parcel, foreign.ID); e != nil || other.Status != "open" || other.ReminderState != "queued" {
		t.Fatalf("foreign task changed %+v %v", other, e)
	}
	if other, e := s.task(ctx, a, otherParcel, unrelated.ID); e != nil || other.Status != "open" || other.ReminderState != "queued" {
		t.Fatalf("other parcel changed %+v %v", other, e)
	}
	var status, state string
	if e = s.DB.QueryRow(ctx, `SELECT t.status,r.state FROM user_parcel_tasks t JOIN user_task_reminders r ON r.user_id=t.user_id AND r.task_id=t.id WHERE t.user_id=$1 AND t.id=$2`, string(a), first.ID).Scan(&status, &state); e != nil || status != "canceled" || state != "canceled" {
		t.Fatalf("clear %s %s %v", status, state, e)
	}
	if e = s.DB.QueryRow(ctx, `SELECT status FROM user_parcel_tasks WHERE user_id=$1 AND id=$2`, string(a), second.ID).Scan(&status); e != nil || status != "completed" {
		t.Fatalf("completed %s %v", status, e)
	}
	if _, e = s.clearParcelTasks(ctx, a, parcel, 1, snapshot); !errors.Is(e, errConflict) {
		t.Fatalf("repeat clear %v", e)
	}
}

func TestTaskThenFavoritePublicRevision(t *testing.T) {
	s, a, _, parcel := fixture(t)
	request := func(method, path, payload string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://app.test"+path, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://app.test")
		w := httptest.NewRecorder()
		s.Serve(w, req, a, "http://app.test")
		return w
	}
	path := "/api/me/favorites/" + parcel
	if w := request("GET", path, ""); w.Code != 404 {
		t.Fatalf("initial favorite %d", w.Code)
	}
	taskPath := "/api/me/parcels/" + parcel + "/tasks"
	due := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339)
	body := fmt.Sprintf(`{"id":"8d1bb391-3008-481a-a94a-3eae8bf62504","message":"Private","due_at":%q,"timezone":"UTC"}`, due)
	if w := request("POST", taskPath, body); w.Code != 200 {
		t.Fatalf("task %d %s", w.Code, w.Body.String())
	}
	w := request("GET", path, "")
	if w.Code != 200 {
		t.Fatalf("favorite read %d %s", w.Code, w.Body.String())
	}
	var current Card
	if e := json.Unmarshal(w.Body.Bytes(), &current); e != nil || current.Favorite || current.Revision < 1 {
		t.Fatalf("retained status %+v %v", current, e)
	}
	w = request("PUT", path, fmt.Sprintf(`{"revision":%d}`, current.Revision))
	if w.Code != 200 {
		t.Fatalf("favorite write %d %s", w.Code, w.Body.String())
	}
	var favorited Card
	if e := json.Unmarshal(w.Body.Bytes(), &favorited); e != nil || !favorited.Favorite {
		t.Fatalf("favorite %+v %v", favorited, e)
	}
	if w := request("PUT", path, `{}`); w.Code != 200 {
		t.Fatalf("active idempotence %d", w.Code)
	}
	if w := request("DELETE", path, fmt.Sprintf(`{"revision":%d}`, favorited.Revision)); w.Code != 200 {
		t.Fatalf("remove %d", w.Code)
	}
	if w := request("PUT", path, `{}`); w.Code != 409 {
		t.Fatalf("stale reactivation %d", w.Code)
	}
}

func TestClearLogsOnlyCommittedQueuedReminders(t *testing.T) {
	s, a, b, parcel := fixture(t)
	ctx := context.Background()
	due := time.Now().Add(3 * time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339)
	create := func(user AccountID, p, id, message string) Task {
		tsk, e := s.createTask(ctx, user, p, id, message, due, "UTC")
		if e != nil {
			t.Fatal(e)
		}
		return tsk
	}
	first := create(a, parcel, "a99f736e-e9e7-4a31-a4bd-a72877dcd046", "SECRET_FIRST")
	second := create(a, parcel, "afba594e-d574-4de1-8aa5-5140fd141247", "SECRET_SECOND")
	second, e := s.changeTask(ctx, a, parcel, second.ID, "reschedule", "SECRET_SECOND", time.Now().Add(4*time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339), "UTC", second.Revision)
	if e != nil {
		t.Fatal(e)
	}
	completed := create(a, parcel, "38557690-62ed-4655-a0da-6c558a0123a4", "SECRET_COMPLETED")
	if _, e := s.changeTask(ctx, a, parcel, completed.ID, "complete", "", "", "", completed.Revision); e != nil {
		t.Fatal(e)
	}
	foreign := create(b, parcel, "f456193d-e399-4bbd-a74f-20dd6285e0dc", "SECRET_FOREIGN")
	var otherParcel string
	if e := s.DB.QueryRow(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,parcel_number) VALUES('union','union','union_county_gis',$1,$1) RETURNING id::text`, "clear-log-"+parcel).Scan(&otherParcel); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		s.DB.Exec(context.Background(), `DELETE FROM user_parcels WHERE user_id=$1 AND parcel_id=$2`, string(a), otherParcel)
		s.DB.Exec(context.Background(), `DELETE FROM parcels WHERE id=$1`, otherParcel)
	})
	unrelated := create(a, otherParcel, "a3caf026-afce-4e54-8d13-57928b1e1ce6", "SECRET_OTHER")
	count, snapshot, e := s.openTaskSnapshot(ctx, a, parcel)
	if e != nil || count != 2 {
		t.Fatalf("snapshot %d %v", count, e)
	}
	var logs bytes.Buffer
	prior := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(prior)
	request := func(method, path, payload string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://app.test"+path, strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "http://app.test")
		w := httptest.NewRecorder()
		s.Serve(w, req, a, "http://app.test")
		return w
	}
	path := "/api/me/parcels/" + parcel + "/tasks/clear"
	if w := request("POST", path, fmt.Sprintf(`{"count":1,"snapshot":%q}`, snapshot)); w.Code != 409 {
		t.Fatalf("conflict %d", w.Code)
	}
	if logs.Len() != 0 {
		t.Fatalf("conflict logged %s", logs.String())
	}
	if current, e := s.task(ctx, a, parcel, first.ID); e != nil || current.Status != "open" || current.ReminderState != "queued" {
		t.Fatalf("conflict mutated %+v %v", current, e)
	}
	if _, e := s.DB.Exec(ctx, `UPDATE user_parcel_tasks SET revision=revision+1 WHERE user_id=$1 AND id=$2`, string(a), first.ID); e != nil {
		t.Fatal(e)
	}
	if w := request("POST", path, fmt.Sprintf(`{"count":%d,"snapshot":%q}`, count, snapshot)); w.Code != 409 || logs.Len() != 0 {
		t.Fatalf("same-count conflict logged %d %s", w.Code, logs.String())
	}
	count, snapshot, e = s.openTaskSnapshot(ctx, a, parcel)
	if e != nil || count != 2 {
		t.Fatalf("refreshed snapshot %d %v", count, e)
	}
	if w := request("POST", path, fmt.Sprintf(`{"count":%d,"snapshot":%q}`, count, snapshot)); w.Code != 200 {
		t.Fatalf("clear %d %s", w.Code, w.Body.String())
	}
	text := logs.String()
	for _, tsk := range []Task{first, second} {
		if strings.Count(text, "reminder cancel-stub task_id="+tsk.ID+" parcel_id="+parcel+" due_utc="+tsk.DueAt.UTC().Format(time.RFC3339Nano)+fmt.Sprintf(" version=%d simulated=true", tsk.ScheduleVersion)) != 1 {
			t.Fatalf("missing or duplicate cancel id=%s log=%s", tsk.ID, text)
		}
	}
	if strings.Count(text, "reminder cancel-stub") != 2 || strings.Contains(text, "SECRET_") || strings.Contains(text, completed.ID) || strings.Contains(text, foreign.ID) || strings.Contains(text, unrelated.ID) {
		t.Fatalf("unexpected cancel logs %s", text)
	}
	for _, tc := range []struct {
		user          AccountID
		p             string
		tsk           Task
		status, state string
	}{{a, parcel, completed, "completed", "canceled"}, {b, parcel, foreign, "open", "queued"}, {a, otherParcel, unrelated, "open", "queued"}} {
		current, e := s.task(ctx, tc.user, tc.p, tc.tsk.ID)
		if e != nil || current.Status != tc.status || current.ReminderState != tc.state {
			t.Fatalf("other task %+v %v", current, e)
		}
	}
	if w := request("POST", path, fmt.Sprintf(`{"count":%d,"snapshot":%q}`, count, snapshot)); w.Code != 409 || logs.String() != text {
		t.Fatalf("repeat mutated or logged %d %s", w.Code, logs.String())
	}
}

func TestClearSerializesWithCreation(t *testing.T) {
	s, a, _, parcel := fixture(t)
	ctx := context.Background()
	due := time.Now().Add(time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339)
	if _, e := s.createTask(ctx, a, parcel, "591cb670-5491-4862-b0d1-6cae84c14443", "One", due, "UTC"); e != nil {
		t.Fatal(e)
	}
	count, snapshot, e := s.openTaskSnapshot(ctx, a, parcel)
	if e != nil || count != 1 {
		t.Fatalf("snapshot %d %v", count, e)
	}
	type result struct {
		n int
		e error
	}
	done := make(chan result, 2)
	go func() { n, e := s.clearParcelTasks(ctx, a, parcel, 1, snapshot); done <- result{n, e} }()
	go func() {
		_, e := s.createTask(ctx, a, parcel, "8bb1516a-8546-4881-a0ea-d48ef65dd1f2", "Two", due, "UTC")
		done <- result{0, e}
	}()
	for i := 0; i < 2; i++ {
		r := <-done
		if r.e != nil && !errors.Is(r.e, errConflict) {
			t.Fatal(r.e)
		}
	}
	var open int
	if e := s.DB.QueryRow(ctx, `SELECT count(*) FROM user_parcel_tasks WHERE user_id=$1 AND parcel_id=$2 AND status='open'`, string(a), parcel).Scan(&open); e != nil {
		t.Fatal(e)
	}
	if open > 0 {
		count, snapshot, e := s.openTaskSnapshot(ctx, a, parcel)
		if e != nil || count != open {
			t.Fatalf("fresh snapshot %d %v", count, e)
		}
		if _, e := s.clearParcelTasks(ctx, a, parcel, open, snapshot); e != nil {
			t.Fatal(e)
		}
	}
}

func TestClearRejectsSameCountChangedTasks(t *testing.T) {
	s, a, _, parcel := fixture(t)
	ctx := context.Background()
	due := time.Now().Add(time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339)
	first, e := s.createTask(ctx, a, parcel, "01b00cda-f568-4863-9481-2d74819521ab", "One", due, "UTC")
	if e != nil {
		t.Fatal(e)
	}
	count, snapshot, e := s.openTaskSnapshot(ctx, a, parcel)
	if e != nil || count != 1 {
		t.Fatalf("snapshot %d %v", count, e)
	}
	if _, e = s.changeTask(ctx, a, parcel, first.ID, "complete", "", "", "", first.Revision); e != nil {
		t.Fatal(e)
	}
	next, e := s.createTask(ctx, a, parcel, "befffa08-428b-4d0a-984c-d83b095c8e54", "Replacement", due, "UTC")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.clearParcelTasks(ctx, a, parcel, 1, snapshot); !errors.Is(e, errConflict) {
		t.Fatalf("same count replacement %v", e)
	}
	fresh, e := s.task(ctx, a, parcel, next.ID)
	if e != nil || fresh.ReminderState != "queued" || fresh.Status != "open" {
		t.Fatalf("replacement changed %+v %v", fresh, e)
	}
	count, snapshot, e = s.openTaskSnapshot(ctx, a, parcel)
	if e != nil || count != 1 {
		t.Fatal(e)
	}
	if _, e = s.clearParcelTasks(ctx, a, parcel, count, snapshot); e != nil {
		t.Fatal(e)
	}
	if _, e = s.changeTask(ctx, a, parcel, next.ID, "stub", "", "", "", next.Revision); !errors.Is(e, errConflict) {
		t.Fatalf("stale version stub %v", e)
	}
}
