package parcelcrm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Task status describes work by the buyer; reminder state describes only simulated delivery intent.
type Task struct {
	ID              string    `json:"id"`
	ParcelID        string    `json:"parcel_id"`
	ParcelNumber    *string   `json:"parcel_number"`
	Message         string    `json:"message"`
	DueAt           time.Time `json:"due_at"`
	Timezone        string    `json:"timezone"`
	Status          string    `json:"status"`
	ReminderState   string    `json:"reminder_state"`
	Revision        int64     `json:"revision"`
	ScheduleVersion int64     `json:"schedule_version"`
}
type taskPage struct {
	Items []Task `json:"items"`
	Next  string `json:"next"`
}

// GSM extension characters consume two septets; any non-GSM character switches the whole SMS to UCS-2.
const gsmBasic = "@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞ ÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà"
const gsmExtension = "^{}\\[~]|€\f"

func validMessage(s string) bool {
	if !utf8.ValidString(s) || strings.TrimSpace(s) == "" {
		return false
	}
	septets := 0
	unicodeUnits := 0
	gsm := true
	for _, r := range s {
		unicodeUnits += len(utf16.Encode([]rune{r}))
		if strings.ContainsRune(gsmBasic, r) {
			septets++
		} else if strings.ContainsRune(gsmExtension, r) {
			septets += 2
		} else {
			gsm = false
		}
	}
	if gsm {
		return septets <= 160
	}
	return unicodeUnits <= 70
}

// Require an offset-bearing local instant and a matching IANA zone. RFC3339 offsets disambiguate fall-back hours.
func parseDue(value, zone string) (time.Time, error) {
	if len(value) > 64 || len(zone) > 100 || zone == "" {
		return time.Time{}, errInvalid
	}
	z, e := time.LoadLocation(zone)
	if e != nil {
		return time.Time{}, errInvalid
	}
	parsed, e := time.Parse(time.RFC3339, value)
	if e != nil {
		return time.Time{}, errInvalid
	}
	local := parsed.In(z)
	// Compare local wall-clock and offset. A skipped hour or a false zone offset fails.
	_, givenOffset := parsed.Zone()
	_, zoneOffset := local.Zone()
	if parsed.Year() != local.Year() || parsed.Month() != local.Month() || parsed.Day() != local.Day() || parsed.Hour() != local.Hour() || parsed.Minute() != local.Minute() || parsed.Second() != local.Second() || givenOffset != zoneOffset {
		return time.Time{}, errInvalid
	}
	return parsed.UTC(), nil
}
func scanTask(row pgx.Row) (Task, error) {
	var t Task
	e := row.Scan(&t.ID, &t.ParcelID, &t.ParcelNumber, &t.Message, &t.DueAt, &t.Timezone, &t.Status, &t.ReminderState, &t.Revision, &t.ScheduleVersion)
	return t, e
}

const taskFields = `t.id::text,t.parcel_id::text,p.parcel_number,t.message,t.due_at,t.timezone,t.status,r.state,t.revision,t.schedule_version`
const taskTables = ` FROM user_parcel_tasks t JOIN user_task_reminders r ON r.user_id=t.user_id AND r.task_id=t.id AND r.version=t.schedule_version JOIN parcels p ON p.id=t.parcel_id `

func (s Store) task(ctx context.Context, user AccountID, parcel, id string) (Task, error) {
	if !uuid(id) || !uuid(parcel) {
		return Task{}, errInvalid
	}
	return scanTask(s.DB.QueryRow(ctx, `SELECT `+taskFields+taskTables+` WHERE t.user_id=$1 AND t.parcel_id=$2 AND t.id=$3`, string(user), parcel, id))
}
func (s Store) tasks(ctx context.Context, user AccountID, parcel, cursor string, limit int) (taskPage, error) {
	page := taskPage{Items: []Task{}}
	if limit < 1 || limit > 50 || (parcel != "" && !uuid(parcel)) {
		return page, errInvalid
	}
	if parcel != "" {
		var stored string
		e := s.DB.QueryRow(ctx, `SELECT id::text FROM parcels WHERE id=$1`, parcel).Scan(&stored)
		if e != nil {
			return page, e
		}
	}
	var at *time.Time
	var id *string
	if cursor != "" {
		parts := strings.Split(cursor, "|")
		if len(parts) != 2 || !uuid(parts[1]) {
			return page, errInvalid
		}
		parsed, e := time.Parse(time.RFC3339Nano, parts[0])
		if e != nil {
			return page, errInvalid
		}
		at = &parsed
		id = &parts[1]
	}
	rows, e := s.DB.Query(ctx, `SELECT `+taskFields+taskTables+` WHERE t.user_id=$1 AND ($2::uuid IS NULL OR t.parcel_id=$2) AND t.status='open' AND ($3::timestamptz IS NULL OR (t.due_at,t.id)>($3::timestamptz,$4::uuid)) ORDER BY t.due_at,t.id LIMIT $5`, string(user), nilIfEmpty(parcel), at, id, limit+1)
	if e != nil {
		return page, e
	}
	defer rows.Close()
	for rows.Next() {
		var t Task
		if e = rows.Scan(&t.ID, &t.ParcelID, &t.ParcelNumber, &t.Message, &t.DueAt, &t.Timezone, &t.Status, &t.ReminderState, &t.Revision, &t.ScheduleVersion); e != nil {
			return page, e
		}
		page.Items = append(page.Items, t)
	}
	if e = rows.Err(); e != nil {
		return page, e
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		last := page.Items[limit-1]
		page.Next = last.DueAt.Format(time.RFC3339Nano) + "|" + last.ID
	}
	return page, nil
}
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func cancelParcelReminders(ctx context.Context, tx pgx.Tx, user AccountID, parcel string) ([]Task, error) {
	rows, e := tx.Query(ctx, `UPDATE user_task_reminders r SET state='canceled',changed_at=now() FROM user_parcel_tasks t WHERE r.user_id=$1 AND r.parcel_id=$2 AND r.state='queued' AND t.user_id=r.user_id AND t.id=r.task_id AND t.schedule_version=r.version RETURNING r.task_id::text,r.version,r.due_at`, string(user), parcel)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		t := Task{ParcelID: parcel}
		if e = rows.Scan(&t.ID, &t.ScheduleVersion, &t.DueAt); e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func logReminder(op string, t Task) {
	log.Printf("reminder %s task_id=%s parcel_id=%s due_utc=%s version=%d simulated=true", op, t.ID, t.ParcelID, t.DueAt.UTC().Format(time.RFC3339Nano), t.ScheduleVersion)
}

// The caller already holds the account row lock. A missing source parcel remains 404.
func (s Store) initTaskParcel(ctx context.Context, tx pgx.Tx, user AccountID, parcel string) error {
	var state string
	err := tx.QueryRow(ctx, `SELECT default_state_id::text FROM user_crm_settings WHERE user_id=$1`, string(user)).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `INSERT INTO user_lead_states(user_id,name,position) VALUES($1,'New',0) RETURNING id::text`, string(user)).Scan(&state)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO user_crm_settings(user_id,default_state_id) VALUES($1,$2)`, string(user), state)
	}
	if err != nil {
		return err
	}
	var inserted string
	err = tx.QueryRow(ctx, `INSERT INTO user_parcels(user_id,parcel_id,state_id,favorite) SELECT $1,id,$3,false FROM parcels WHERE id=$2 ON CONFLICT (user_id,parcel_id) DO NOTHING RETURNING parcel_id::text`, string(user), parcel, state).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.QueryRow(ctx, `SELECT parcel_id::text FROM user_parcels WHERE user_id=$1 AND parcel_id=$2`, string(user), parcel).Scan(&inserted)
	}
	return err
}

// The snapshot records the exact open task IDs and revisions, not only a count.
// LIMIT bounds confirmation reads; no mutation occurs above 5000 open tasks.
const taskSnapshotSQL = `SELECT count(*)::int, md5(coalesce(string_agg(id::text || ':' || revision::text, ',' ORDER BY id),'')) FROM (SELECT id,revision FROM user_parcel_tasks WHERE user_id=$1 AND parcel_id=$2 AND status='open' ORDER BY id LIMIT 5001) open_tasks`

func (s Store) openTaskSnapshot(ctx context.Context, user AccountID, parcel string) (int, string, error) {
	if !uuid(parcel) {
		return 0, "", errInvalid
	}
	var id string
	if e := s.DB.QueryRow(ctx, `SELECT parcel_id::text FROM user_parcels WHERE user_id=$1 AND parcel_id=$2`, string(user), parcel).Scan(&id); e != nil {
		return 0, "", e
	}
	var count int
	var snapshot string
	err := s.DB.QueryRow(ctx, taskSnapshotSQL, string(user), parcel).Scan(&count, &snapshot)
	if count > 5000 {
		return 0, "", errInvalid
	}
	return count, snapshot, err
}
func (s Store) clearParcelTasks(ctx context.Context, user AccountID, parcel string, expected int, expectedSnapshot string) (int, error) {
	if !uuid(parcel) || expected < 1 || expected > 5000 || len(expectedSnapshot) != 32 {
		return 0, errInvalid
	}
	for _, r := range expectedSnapshot {
		if r < '0' || r > '9' {
			if r < 'a' || r > 'f' {
				return 0, errInvalid
			}
		}
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback(ctx)
	var id string
	if e = tx.QueryRow(ctx, `SELECT id::text FROM app_users WHERE id=$1 FOR UPDATE`, string(user)).Scan(&id); e != nil {
		return 0, e
	}
	if e = tx.QueryRow(ctx, `SELECT parcel_id::text FROM user_parcels WHERE user_id=$1 AND parcel_id=$2`, string(user), parcel).Scan(&id); e != nil {
		return 0, e
	}
	var count int
	var snapshot string
	if e = tx.QueryRow(ctx, taskSnapshotSQL, string(user), parcel).Scan(&count, &snapshot); e != nil {
		return 0, e
	}
	if count != expected || snapshot != expectedSnapshot {
		return 0, errConflict
	}
	// All supported task writes acquire the account lock first. Concurrent creations
	// before this lock invalidate the snapshot; creations after commit stay open.
	rows, e := tx.Query(ctx, `UPDATE user_task_reminders r SET state='canceled',changed_at=now() FROM user_parcel_tasks t WHERE t.user_id=$1 AND t.parcel_id=$2 AND t.status='open' AND r.user_id=t.user_id AND r.task_id=t.id AND r.version=t.schedule_version AND r.state='queued' RETURNING r.task_id::text,r.version,r.due_at`, string(user), parcel)
	if e != nil {
		return 0, e
	}
	var canceled []Task
	for rows.Next() {
		reminder := Task{ParcelID: parcel}
		if e = rows.Scan(&reminder.ID, &reminder.ScheduleVersion, &reminder.DueAt); e != nil {
			rows.Close()
			return 0, e
		}
		canceled = append(canceled, reminder)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	tag, e := tx.Exec(ctx, `UPDATE user_parcel_tasks SET status='canceled',revision=revision+1,updated_at=now() WHERE user_id=$1 AND parcel_id=$2 AND status='open'`, string(user), parcel)
	if e != nil {
		return 0, e
	}
	if tag.RowsAffected() != int64(count) {
		return 0, errConflict
	}
	if e = tx.Commit(ctx); e != nil {
		return 0, e
	}
	for _, reminder := range canceled {
		logReminder("cancel-stub", reminder)
	}
	return count, nil
}
func (s Store) createTask(ctx context.Context, user AccountID, parcel, id, message, due, zone string) (Task, error) {
	if !uuid(parcel) || !uuid(id) || !validMessage(message) {
		return Task{}, errInvalid
	}
	instant, e := parseDue(due, zone)
	if e != nil {
		return Task{}, e
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return Task{}, e
	}
	defer tx.Rollback(ctx)
	var locked string
	if e = tx.QueryRow(ctx, `SELECT id::text FROM app_users WHERE id=$1 FOR UPDATE`, string(user)).Scan(&locked); e != nil {
		return Task{}, e
	}
	// Retry tokens are globally scoped to the account, not to another parcel or changed payload.
	var existing Task
	e = tx.QueryRow(ctx, `SELECT `+taskFields+taskTables+` WHERE t.user_id=$1 AND t.id=$2`, string(user), id).Scan(&existing.ID, &existing.ParcelID, &existing.ParcelNumber, &existing.Message, &existing.DueAt, &existing.Timezone, &existing.Status, &existing.ReminderState, &existing.Revision, &existing.ScheduleVersion)
	if e == nil {
		if existing.ParcelID != parcel || existing.Message != message || !existing.DueAt.Equal(instant) || existing.Timezone != zone {
			return Task{}, errConflict
		}
		return existing, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return Task{}, e
	}
	if instant.Before(time.Now()) {
		return Task{}, errInvalid
	}
	if e = s.initTaskParcel(ctx, tx, user, parcel); e != nil {
		return Task{}, e
	}
	var owned string
	if e = tx.QueryRow(ctx, `SELECT parcel_id::text FROM user_parcels WHERE user_id=$1 AND parcel_id=$2 AND NOT excluded FOR UPDATE`, string(user), parcel).Scan(&owned); e != nil {
		return Task{}, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO user_parcel_tasks(user_id,parcel_id,id,message,due_at,timezone,status) VALUES($1,$2,$3,$4,$5,$6,'open')`, string(user), parcel, id, message, instant, zone)
	if e != nil {
		return Task{}, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO user_task_reminders(user_id,parcel_id,task_id,version,due_at,state) VALUES($1,$2,$3,1,$4,'queued')`, string(user), parcel, id, instant)
	if e != nil {
		return Task{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Task{}, e
	}
	logReminder("schedule-stub", Task{ID: id, ParcelID: parcel, DueAt: instant, ScheduleVersion: 1})
	return s.task(ctx, user, parcel, id)
}

// All changes serialize on the account row, matching favorite removal and exclusion.
func (s Store) changeTask(ctx context.Context, user AccountID, parcel, id, action, message, due, zone string, revision int64) (Task, error) {
	if !uuid(parcel) || !uuid(id) || revision < 1 {
		return Task{}, errInvalid
	}
	var instant time.Time
	var e error
	if action == "reschedule" {
		if !validMessage(message) {
			return Task{}, errInvalid
		}
		instant, e = parseDue(due, zone)
		if e != nil {
			return Task{}, e
		}
		if instant.Before(time.Now()) {
			return Task{}, errInvalid
		}
	}
	if action != "reschedule" && action != "complete" && action != "cancel" && action != "stub" {
		return Task{}, errInvalid
	}
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return Task{}, e
	}
	defer tx.Rollback(ctx)
	var locked string
	if e = tx.QueryRow(ctx, `SELECT id::text FROM app_users WHERE id=$1 FOR UPDATE`, string(user)).Scan(&locked); e != nil {
		return Task{}, e
	}
	var currentRevision, version int64
	var status string
	var excluded bool
	var oldDue time.Time
	e = tx.QueryRow(ctx, `SELECT t.revision,t.schedule_version,t.status,p.excluded,t.due_at FROM user_parcel_tasks t JOIN user_parcels p ON p.user_id=t.user_id AND p.parcel_id=t.parcel_id WHERE t.user_id=$1 AND t.parcel_id=$2 AND t.id=$3 FOR UPDATE OF t,p`, string(user), parcel, id).Scan(&currentRevision, &version, &status, &excluded, &oldDue)
	if e != nil {
		return Task{}, e
	}
	if revision != currentRevision || status != "open" {
		return Task{}, errConflict
	}
	if action == "reschedule" || action == "stub" {
		if excluded {
			return Task{}, errConflict
		}
	}
	canceledQueued := false
	stubState := ""
	if action == "stub" {
		if time.Now().Before(oldDue) {
			return Task{}, errInvalid
		}
		state := "stubbed"
		if time.Since(oldDue) > time.Hour {
			state = "missed"
		}
		stubState = state
		tag, err := tx.Exec(ctx, `UPDATE user_task_reminders SET state=$4,changed_at=now() WHERE user_id=$1 AND task_id=$2 AND version=$3 AND state='queued'`, string(user), id, version, state)
		if err != nil {
			return Task{}, err
		}
		if tag.RowsAffected() != 1 {
			return Task{}, errConflict
		}
		_, e = tx.Exec(ctx, `UPDATE user_parcel_tasks SET revision=revision+1,updated_at=now() WHERE user_id=$1 AND id=$2`, string(user), id)
	} else if action == "reschedule" {
		var tag pgconn.CommandTag
		tag, e = tx.Exec(ctx, `UPDATE user_task_reminders SET state='canceled',changed_at=now() WHERE user_id=$1 AND task_id=$2 AND version=$3 AND state='queued'`, string(user), id, version)
		canceledQueued = tag.RowsAffected() > 0
		if e == nil {
			_, e = tx.Exec(ctx, `UPDATE user_parcel_tasks SET message=$3,due_at=$4,timezone=$5,schedule_version=schedule_version+1,revision=revision+1,updated_at=now() WHERE user_id=$1 AND id=$2`, string(user), id, message, instant, zone)
		}
		if e == nil {
			_, e = tx.Exec(ctx, `INSERT INTO user_task_reminders(user_id,parcel_id,task_id,version,due_at,state) VALUES($1,$2,$3,$4,$5,'queued')`, string(user), parcel, id, version+1, instant)
		}
	} else {
		var tag pgconn.CommandTag
		tag, e = tx.Exec(ctx, `UPDATE user_task_reminders SET state='canceled',changed_at=now() WHERE user_id=$1 AND task_id=$2 AND version=$3 AND state='queued'`, string(user), id, version)
		canceledQueued = tag.RowsAffected() > 0
		if e == nil {
			_, e = tx.Exec(ctx, `UPDATE user_parcel_tasks SET status=$3,revision=revision+1,updated_at=now() WHERE user_id=$1 AND id=$2`, string(user), id, map[string]string{"complete": "completed", "cancel": "canceled"}[action])
		}
	}
	if e != nil {
		return Task{}, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Task{}, e
	}
	previous := Task{ID: id, ParcelID: parcel, DueAt: oldDue, ScheduleVersion: version}
	if canceledQueued {
		logReminder("cancel-stub", previous)
	}
	switch action {
	case "stub":
		logReminder("sms-"+stubState+"-not-sent", previous)
	case "reschedule":
		logReminder("schedule-stub", Task{ID: id, ParcelID: parcel, DueAt: instant, ScheduleVersion: version + 1})
	}
	return s.task(ctx, user, parcel, id)
}
func (s Store) serveTasks(w http.ResponseWriter, r *http.Request, user AccountID, parts []string) {
	ctx := r.Context()
	var e error
	if len(parts) == 2 && parts[0] == "crm" && parts[1] == "tasks" && r.Method == "GET" {
		limit := 20
		if r.URL.Query().Has("limit") {
			limit, e = strconv.Atoi(r.URL.Query().Get("limit"))
			if e != nil {
				failure(w, errInvalid)
				return
			}
		}
		var page taskPage
		page, e = s.tasks(ctx, user, "", r.URL.Query().Get("cursor"), limit)
		if e == nil {
			send(w, page)
		}
	} else if len(parts) == 4 && parts[0] == "parcels" && uuid(parts[1]) && parts[2] == "tasks" && parts[3] == "clear" {
		parcel := parts[1]
		if r.Method == "GET" {
			var count int
			var snapshot string
			count, snapshot, e = s.openTaskSnapshot(ctx, user, parcel)
			if e == nil {
				send(w, struct {
					Count    int    `json:"count"`
					Snapshot string `json:"snapshot"`
				}{count, snapshot})
			}
		} else if r.Method == "POST" {
			var req struct {
				Count    int    `json:"count"`
				Snapshot string `json:"snapshot"`
			}
			if e = decodeTask(w, r, &req); e == nil {
				var count int
				count, e = s.clearParcelTasks(ctx, user, parcel, req.Count, req.Snapshot)
				if e == nil {
					send(w, struct {
						Canceled int `json:"canceled"`
					}{count})
				}
			}
		} else {
			http.NotFound(w, r)
			return
		}
	} else if len(parts) >= 3 && parts[0] == "parcels" && uuid(parts[1]) && parts[2] == "tasks" && len(parts) <= 5 {
		parcel := parts[1]
		switch {
		case len(parts) == 3 && r.Method == "GET":
			limit := 20
			if r.URL.Query().Has("limit") {
				limit, e = strconv.Atoi(r.URL.Query().Get("limit"))
				if e != nil {
					failure(w, errInvalid)
					return
				}
			}
			var page taskPage
			page, e = s.tasks(ctx, user, parcel, r.URL.Query().Get("cursor"), limit)
			if e == nil {
				send(w, page)
			}
		case len(parts) == 3 && r.Method == "POST":
			var req struct {
				ID       string `json:"id"`
				Message  string `json:"message"`
				DueAt    string `json:"due_at"`
				Timezone string `json:"timezone"`
			}
			if e = decodeTask(w, r, &req); e == nil {
				var t Task
				t, e = s.createTask(ctx, user, parcel, req.ID, req.Message, req.DueAt, req.Timezone)
				if e == nil {
					send(w, t)
				}
			}
		case len(parts) == 4 && uuid(parts[3]) && r.Method == "GET":
			var t Task
			t, e = s.task(ctx, user, parcel, parts[3])
			if e == nil {
				send(w, t)
			}
		case len(parts) == 5 && uuid(parts[3]) && r.Method == "POST":
			var req struct {
				Revision int64  `json:"revision"`
				Message  string `json:"message"`
				DueAt    string `json:"due_at"`
				Timezone string `json:"timezone"`
			}
			if e = decodeTask(w, r, &req); e == nil {
				var t Task
				t, e = s.changeTask(ctx, user, parcel, parts[3], parts[4], req.Message, req.DueAt, req.Timezone, req.Revision)
				if e == nil {
					send(w, t)
				}
			}
		default:
			http.NotFound(w, r)
			return
		}
	} else {
		http.NotFound(w, r)
		return
	}
	if e != nil {
		failure(w, e)
	}
}
func decodeTask(w http.ResponseWriter, r *http.Request, out any) error {
	if r.Header.Get("Content-Type") != "application/json" {
		return errInvalid
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return errInvalid
	}
	var extra any
	if !errors.Is(d.Decode(&extra), io.EOF) {
		return errInvalid
	}
	return nil
}
