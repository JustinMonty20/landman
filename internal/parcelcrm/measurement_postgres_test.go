//go:build postgres

package parcelcrm

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// The opt-in fixture measures local database/JSON work, not network or product budgets.
func TestBoardFixtureMeasurement(t *testing.T) {
	s, a, b, id := fixture(t)
	ctx := context.Background()
	stamp := fmt.Sprintf("crm-measure-%d", time.Now().UnixNano())
	defer func() {
		s.DB.Exec(ctx, `DELETE FROM app_users WHERE id IN ($1,$2)`, string(a), string(b))
		s.DB.Exec(ctx, `DELETE FROM parcels WHERE source_name='crm_measure' AND source_parcel_id LIKE $1||'%'`, stamp)
	}()
	_ = id
	for _, user := range []AccountID{a, b} {
		if e := s.init(ctx, user); e != nil {
			t.Fatal(e)
		}
		for i := 1; i < 50; i++ {
			if _, e := s.state(ctx, user, "", "create", fmt.Sprintf("State %02d", i), "", 0); e != nil {
				t.Fatal(e)
			}
		}
	}
	_, e := s.DB.Exec(ctx, `INSERT INTO parcels(county_id,county_code,source_name,source_parcel_id,parcel_number) SELECT 'union','union','crm_measure',$1||'-'||g,$1||'-'||g FROM generate_series(1,1000) g`, stamp)
	if e != nil {
		t.Fatal(e)
	}
	for _, user := range []AccountID{a, b} {
		_, e = s.DB.Exec(ctx, `INSERT INTO user_parcels(user_id,parcel_id,state_id) SELECT $1,p.id,st.id FROM parcels p JOIN user_lead_states st ON st.user_id=$1 AND st.position=(substring(p.source_parcel_id from '[0-9]+$')::int % 50) WHERE p.source_name='crm_measure' AND p.source_parcel_id LIKE $2||'%'`, string(user), stamp)
		if e != nil {
			t.Fatal(e)
		}
	}
	for _, user := range []AccountID{a, b} {
		start := time.Now()
		board, e := s.board(ctx, user)
		elapsed := time.Since(start)
		if e != nil || len(board.States) != 50 {
			t.Fatalf("board %+v %v", board, e)
		}
		body, _ := json.Marshal(board)
		if board.Next == "" {
			t.Fatal("missing bounded state cursor")
		}
		remaining, e := s.boardPage(ctx, user, board.Next)
		if e != nil || len(remaining.States) != 0 {
			t.Fatalf("state cursor %+v %v", remaining, e)
		}
		start = time.Now()
		cards, e := s.cards(ctx, user, board.States[0].ID, "", 20, false)
		pageElapsed := time.Since(start)
		if e != nil || len(cards) != 20 {
			t.Fatalf("page %d %v", len(cards), e)
		}
		page, _ := json.Marshal(cards)
		t.Logf("LOCAL measured account=%s states=%d favorites=1000 board=%s board_bytes=%d page20=%s page_bytes=%d", user, len(board.States), elapsed, len(body), pageElapsed, len(page))
	}
}
