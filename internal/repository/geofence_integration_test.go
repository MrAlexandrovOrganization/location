package repository_test

import (
	"context"
	"crypto/sha256"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	pb "location/gen/location/v1"
	"location/internal/geofence"
	"location/internal/migrate"
	"location/internal/model"
	"location/internal/repository"
	"location/migrations"
)

func TestGeofencesDurableTransitions(t *testing.T) {
	db := isolatedPostgres(t)
	ctx := t.Context()
	if err := migrate.Run(ctx, db, migrations.FS); err != nil {
		t.Fatal(err)
	}
	store := &geofence.Store{Pool: db}
	server := &geofence.Server{Store: store}
	token := "synthetic-geo-test-token"
	hash := sha256.Sum256([]byte(token))
	listener := bufconn.Listen(1 << 20)
	g := grpc.NewServer(grpc.UnaryInterceptor(geofence.Auth(hash[:])))
	pb.RegisterGeofencesServiceServer(g, server)
	go g.Serve(listener)
	t.Cleanup(g.Stop)
	conn, err := grpc.NewClient("passthrough:///geo-test", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := pb.NewGeofencesServiceClient(conn)
	if _, err = client.ListPlaces(ctx, &pb.ListPlacesRequest{}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing auth: %v", err)
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	request := &pb.CreatePlaceRequest{RequestId: uuid.NewString(), Place: &pb.Place{Name: "Synthetic home", Latitude: 0, Longitude: 0, RadiusM: 150, ConfirmationSeconds: 60}}
	out, err := client.CreatePlace(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	again, err := client.CreatePlace(ctx, request)
	if err != nil || again.Place.Id != out.Place.Id {
		t.Fatalf("lost acknowledgement: %v %v", again, err)
	}
	place := out.Place
	start := time.Now().UTC().Add(time.Second).Truncate(time.Microsecond)
	repo := repository.NewPostgres(db)
	save := func(seconds int, lon float64, reason model.QualityReason) {
		t.Helper()
		at := start.Add(time.Duration(seconds) * time.Second)
		point := &model.Location{ID: uuid.NewString(), Latitude: 0, Longitude: lon, Accuracy: 5, Date: at.Format("2006-01-02"), Source: "android", RecordedAt: at, QualityReason: reason}
		if err := repo.Save(ctx, point); err != nil {
			t.Fatal(err)
		}
		if err := store.Process(ctx, at); err != nil {
			t.Fatal(err)
		}
	}
	save(0, 0, "")
	save(60, 0, "")
	save(120, .004, "")
	// A fresh store instance resumes the pending candidate after a process restart.
	store = &geofence.Store{Pool: db}
	save(180, .004, "")
	feed, err := client.ListEvents(ctx, &pb.ListEventsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(feed.Events) != 1 || feed.Events[0].Transition != "exited" {
		t.Fatalf("events: %v", feed)
	}
	if err = store.Process(ctx, start.Add(180*time.Second)); err != nil {
		t.Fatal(err)
	}
	save(240, 0, model.PoorAccuracy)
	save(300, 0, model.StaleMeasurement)
	// A long gap resumes without inventing an arrival.
	save(900, 0, "")
	save(960, 0, "")
	feed, err = client.ListEvents(ctx, &pb.ListEventsRequest{AfterSequence: feed.NextSequence})
	if err != nil || len(feed.Events) != 0 {
		t.Fatalf("unexpected replay: %v %v", feed, err)
	}
	// Delayed history stays in locations, but cannot produce a live event.
	historical := &model.Location{ID: uuid.NewString(), Latitude: 0, Longitude: .004, Accuracy: 5, Date: "2026-01-01", Source: "android", RecordedAt: start.Add(-time.Hour)}
	if err = repo.Save(ctx, historical); err != nil {
		t.Fatal(err)
	}
	if err = store.Process(ctx, start.Add(960*time.Second)); err != nil {
		t.Fatal(err)
	}
	var queued int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM geo_pending").Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("queue: %d %v", queued, err)
	}
	place.RadiusM = 200
	changed, err := client.UpdatePlace(ctx, &pb.UpdatePlaceRequest{Place: place})
	if err != nil || changed.Place.State != "unknown" || changed.Place.Version != 2 {
		t.Fatalf("reset: %v %v", changed, err)
	}
	if _, err = client.DeletePlace(ctx, &pb.DeletePlaceRequest{Id: place.Id}); err != nil {
		t.Fatal(err)
	}
	places, err := client.ListPlaces(ctx, &pb.ListPlacesRequest{})
	if err != nil || len(places.Places) != 0 {
		t.Fatalf("delete: %v %v", places, err)
	}
}
