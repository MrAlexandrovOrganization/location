package geofence

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	pb "location/gen/location/v1"
)

type Server struct {
	pb.UnimplementedGeofencesServiceServer
	Store *Store
}

func Auth(hash []byte) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		values := metadata.ValueFromIncomingContext(ctx, "authorization")
		if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
			return nil, status.Error(codes.Unauthenticated, "authentication required")
		}
		sum := sha256.Sum256([]byte(strings.TrimPrefix(values[0], "Bearer ")))
		if len(hash) != 32 || subtle.ConstantTimeCompare(sum[:], hash) != 1 {
			return nil, status.Error(codes.Unauthenticated, "authentication required")
		}
		return next(ctx, req)
	}
}
func validatePlace(p *pb.Place, update bool) error {
	if p == nil {
		return status.Error(codes.InvalidArgument, "place required")
	}
	p.Name = strings.TrimSpace(p.Name)
	if utf8.RuneCountInString(p.Name) < 1 || utf8.RuneCountInString(p.Name) > 80 || !utf8.ValidString(p.Name) || strings.ContainsAny(p.Name, "\r\n") {
		return status.Error(codes.InvalidArgument, "invalid place name")
	}
	for _, v := range []float64{p.Latitude, p.Longitude, p.RadiusM} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return status.Error(codes.InvalidArgument, "invalid geometry")
		}
	}
	if p.Latitude < -90 || p.Latitude > 90 || p.Longitude < -180 || p.Longitude > 180 || p.RadiusM < 100 || p.RadiusM > 10000 {
		return status.Error(codes.InvalidArgument, "invalid geometry")
	}
	if p.ConfirmationSeconds == 0 {
		p.ConfirmationSeconds = 60
	}
	if p.ConfirmationSeconds < 30 || p.ConfirmationSeconds > 600 {
		return status.Error(codes.InvalidArgument, "invalid confirmation duration")
	}
	if update {
		if _, err := uuid.Parse(p.Id); err != nil {
			return status.Error(codes.InvalidArgument, "invalid place id")
		}
	}
	return nil
}
func publicError(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := status.FromError(err); ok {
		return err
	}
	return status.Error(codes.Unavailable, "geofences temporarily unavailable")
}
func (s *Server) CreatePlace(ctx context.Context, r *pb.CreatePlaceRequest) (*pb.CreatePlaceResponse, error) {
	if _, err := uuid.Parse(r.GetRequestId()); err != nil {
		return nil, status.Error(codes.InvalidArgument, "request id required")
	}
	if err := validatePlace(r.GetPlace(), false); err != nil {
		return nil, err
	}
	p, err := s.Store.SavePlace(ctx, r.Place, true, r.RequestId)
	return &pb.CreatePlaceResponse{Place: p}, publicError(err)
}
func (s *Server) UpdatePlace(ctx context.Context, r *pb.UpdatePlaceRequest) (*pb.UpdatePlaceResponse, error) {
	if err := validatePlace(r.GetPlace(), true); err != nil {
		return nil, err
	}
	p, err := s.Store.SavePlace(ctx, r.Place, false, "")
	return &pb.UpdatePlaceResponse{Place: p}, publicError(err)
}
func (s *Server) DeletePlace(ctx context.Context, r *pb.DeletePlaceRequest) (*pb.DeletePlaceResponse, error) {
	if _, err := uuid.Parse(r.GetId()); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid place id")
	}
	return &pb.DeletePlaceResponse{}, publicError(s.Store.Delete(ctx, r.Id))
}
func (s *Server) ListPlaces(ctx context.Context, _ *pb.ListPlacesRequest) (*pb.ListPlacesResponse, error) {
	p, err := s.Store.Places(ctx)
	return &pb.ListPlacesResponse{Places: p}, publicError(err)
}
func (s *Server) ListEvents(ctx context.Context, r *pb.ListEventsRequest) (*pb.ListEventsResponse, error) {
	if r.GetAfterSequence() < 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid cursor")
	}
	limit := r.GetLimit()
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 500 {
		return nil, status.Error(codes.InvalidArgument, "invalid limit")
	}
	out, err := s.Store.Events(ctx, r.GetAfterSequence(), limit)
	return out, publicError(err)
}
