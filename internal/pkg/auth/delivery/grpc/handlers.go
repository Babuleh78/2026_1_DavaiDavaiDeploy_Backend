package grpc

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/auth"
	"DDDance/internal/pkg/auth/delivery/grpc/gen"
	"context"
	"os"

	uuid "github.com/satori/go.uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type GrpcAuthHandler struct {
	JWTSecret string
	auc       auth.AuthUsecase
	gen.UnimplementedAuthServer
}

func NewGrpcAuthHandler(auc auth.AuthUsecase) *GrpcAuthHandler {
	return &GrpcAuthHandler{auc: auc, JWTSecret: os.Getenv("JWT_SECRET")}
}

func (g GrpcAuthHandler) SignupUser(ctx context.Context, in *gen.SignupRequest) (*gen.AuthResponse, error) {
	req := models.SignUpInput{
		Login:    in.Login,
		Password: in.Password,
	}
	req.Sanitize()

	user, token, err := g.auc.SignUpUser(ctx, req)
	if err != nil {
		switch err {
		case auth.ErrorBadRequest:
			return nil, status.Errorf(codes.InvalidArgument, "%v", err)
		case auth.ErrorConflict:
			return nil, status.Errorf(codes.AlreadyExists, "%v", err)
		default:
			return nil, status.Errorf(codes.Internal, "%v", err)
		}
	}
	user.Sanitize()

	csrfToken := uuid.NewV4().String()

	userResponse := &gen.UserResponse{
		ID:      user.ID.String(),
		Version: int32(user.Version),
		Login:   user.Login,
		Avatar:  user.Avatar,
	}

	return &gen.AuthResponse{
		User:      userResponse,
		JWTToken:  token,
		CSRFToken: csrfToken,
	}, err
}

func (g GrpcAuthHandler) SignInUser(ctx context.Context, in *gen.SignInRequest) (*gen.AuthResponse, error) {
	req := models.SignInInput{
		Login:    in.Login,
		Password: in.Password,
		Code:     in.TwoFactorCode,
	}
	req.Sanitize()

	user, token, err := g.auc.SignInUser(ctx, req)
	if err != nil {
		switch err {
		case auth.ErrorBadRequest:
			return nil, status.Errorf(codes.InvalidArgument, "%v", err)
		case auth.ErrorUnauthorized:
			return nil, status.Errorf(codes.Unauthenticated, "%v", err)
		case auth.ErrorPreconditionFailed:
			return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
		default:
			return nil, status.Errorf(codes.Internal, "%v", err)
		}
	}
	user.Sanitize()

	userResponse := &gen.UserResponse{
		ID:      user.ID.String(),
		Version: int32(user.Version),
		Login:   user.Login,
		Avatar:  user.Avatar,
		Has2Fa:  user.Has2FA,
	}

	csrfToken := uuid.NewV4().String()

	return &gen.AuthResponse{
		User:      userResponse,
		JWTToken:  token,
		CSRFToken: csrfToken,
	}, err
}

func (g GrpcAuthHandler) LogOutUser(ctx context.Context, in *gen.LogOutUserRequest) (*gen.LogOutUserResponse, error) {
	err := g.auc.LogOutUser(ctx, uuid.FromStringOrNil(in.ID))
	if err != nil {
		switch err {
		case auth.ErrorUnauthorized:
			return nil, status.Errorf(codes.Unauthenticated, "%v", err)
		default:
			return nil, status.Errorf(codes.Internal, "%v", err)
		}
	}
	response := &gen.LogOutUserResponse{}
	return response, nil
}

func (g GrpcAuthHandler) ValidateAndGetUser(ctx context.Context, in *gen.ValidateAndGetUserRequest) (*gen.UserResponse, error) {
	user, err := g.auc.ValidateAndGetUser(ctx, in.Token)
	if err != nil {
		switch err {
		case auth.ErrorUnauthorized:
			return nil, status.Errorf(codes.Unauthenticated, "%v", err)
		default:
			return nil, status.Errorf(codes.Internal, "%v", err)
		}
	}
	user.Sanitize()

	return &gen.UserResponse{
		ID:        user.ID.String(),
		Version:   int32(user.Version),
		Login:     user.Login,
		Avatar:    user.Avatar,
		Has2Fa:    user.Has2FA,
		IsForeign: user.IsForeign,
	}, err
}
