package geofence

import (
	"context"
	"embed"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	pb "location/gen/location/v1"
)

//go:embed queries/*.sql
var queries embed.FS

func query(name string) string {
	b, err := queries.ReadFile("queries/" + name + ".sql")
	if err != nil {
		panic(err)
	}
	return string(b)
}

type Store struct{ Pool *pgxpool.Pool }

func (s *Store) SavePlace(ctx context.Context, p *pb.Place, create bool, requestID string) (*pb.Place, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, query("lock")); err != nil {
		return nil, err
	}
	p = proto.Clone(p).(*pb.Place)
	name := "update"
	if create {
		name = "create"
		p.Id = requestID
	}
	old := new(pb.Place)
	var at *time.Time
	var enabled bool
	err = tx.QueryRow(ctx, query("place"), p.Id).Scan(&old.Id, &old.Name, &old.Latitude, &old.Longitude, &old.RadiusM, &old.ConfirmationSeconds, &old.Version, &old.State, &at, &enabled)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		same := old.Name == p.Name && old.Latitude == p.Latitude && old.Longitude == p.Longitude && old.RadiusM == p.RadiusM && old.ConfirmationSeconds == p.ConfirmationSeconds
		if create && (!same || !enabled) {
			return nil, status.Error(codes.AlreadyExists, "request id already used")
		}
		if !enabled {
			return nil, status.Error(codes.NotFound, "place not found")
		}
		if same {
			if at != nil {
				old.LastObservedAt = timestamppb.New(*at)
			}
			return old, tx.Commit(ctx)
		}
	}
	err = tx.QueryRow(ctx, query(name), p.Id, p.Name, p.Latitude, p.Longitude, p.RadiusM, p.ConfirmationSeconds).Scan(&p.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		if create {
			return nil, status.Error(codes.ResourceExhausted, "place limit reached")
		}
		return nil, status.Error(codes.NotFound, "place not found")
	}
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, query("state_reset"), p.Id); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	p.State = "unknown"
	p.LastObservedAt = nil
	return p, nil
}
func (s *Store) Delete(ctx context.Context, id string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, query("lock")); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, query("delete"), id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return status.Error(codes.NotFound, "place not found")
	}
	return tx.Commit(ctx)
}
func (s *Store) Places(ctx context.Context) ([]*pb.Place, error) {
	rows, err := s.Pool.Query(ctx, query("places"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*pb.Place
	for rows.Next() {
		p := new(pb.Place)
		var at *time.Time
		if err = rows.Scan(&p.Id, &p.Name, &p.Latitude, &p.Longitude, &p.RadiusM, &p.ConfirmationSeconds, &p.Version, &p.State, &at); err != nil {
			return nil, err
		}
		if at != nil {
			p.LastObservedAt = timestamppb.New(*at)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Store) Events(ctx context.Context, after int64, limit int32) (*pb.ListEventsResponse, error) {
	rows, err := s.Pool.Query(ctx, query("events"), after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &pb.ListEventsResponse{NextSequence: after}
	for rows.Next() {
		e := new(pb.PlaceEvent)
		var occurred, confirmed time.Time
		if err = rows.Scan(&e.Sequence, &e.PlaceId, &e.PlaceVersion, &e.Transition, &occurred, &confirmed); err != nil {
			return nil, err
		}
		e.OccurredAt = timestamppb.New(occurred)
		e.ConfirmedAt = timestamppb.New(confirmed)
		out.Events = append(out.Events, e)
		out.NextSequence = e.Sequence
	}
	return out, rows.Err()
}

// Process commits state, events and evaluation acknowledgements atomically.
// The shared transaction lock also serializes place edits and event sequence commits.
func (s *Store) Process(ctx context.Context, now time.Time) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, query("lock")); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, query("prune"), now); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, query("pending"), now)
	if err != nil {
		return err
	}
	var fixes []Fix
	for rows.Next() {
		var f Fix
		if err = rows.Scan(&f.ID, &f.Latitude, &f.Longitude, &f.Accuracy, &f.At); err != nil {
			rows.Close()
			return err
		}
		fixes = append(fixes, f)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, f := range fixes {
		if err = s.processFix(ctx, tx, f); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, query("pending_delete"), f.ID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) processFix(ctx context.Context, tx pgx.Tx, f Fix) error {
	rows, err := tx.Query(ctx, query("states"), f.At)
	if err != nil {
		return err
	}
	type entry struct {
		id               string
		lat, lon, radius float64
		seconds          int32
		version          int64
		state            State
	}
	var entries []entry
	for rows.Next() {
		var p entry
		if err = rows.Scan(&p.id, &p.lat, &p.lon, &p.radius, &p.seconds, &p.version, &p.state.Stable, &p.state.Candidate, &p.state.Since, &p.state.Last, &p.state.LocationID); err != nil {
			rows.Close()
			return err
		}
		entries = append(entries, p)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, p := range entries {
		next, event, since := Advance(p.state, p.lat, p.lon, p.radius, time.Duration(p.seconds)*time.Second, f)
		if _, err = tx.Exec(ctx, query("state_save"), p.id, next.Stable, next.Candidate, next.Since, next.Last, next.LocationID); err != nil {
			return err
		}
		if event != "" {
			if _, err = tx.Exec(ctx, query("event_add"), p.id, p.version, event, since, f.At); err != nil {
				return err
			}
		}
	}
	return nil
}
