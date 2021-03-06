package sentry

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/fabric8-services/fabric8-common/login/tokencontext"
	"github.com/fabric8-services/fabric8-common/resource"
	testtoken "github.com/fabric8-services/fabric8-common/test/token"
	"github.com/fabric8-services/fabric8-common/token"

	"github.com/dgrijalva/jwt-go"
	"github.com/getsentry/raven-go"
	goajwt "github.com/goadesign/goa/middleware/security/jwt"
	"github.com/pkg/errors"
	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func failOnNoToken(t *testing.T) context.Context {
	// this is just normal context object with no, token
	// so this should fail saying no token available
	m := testtoken.NewManager()
	return tokencontext.ContextWithTokenManager(context.Background(), m)
}

func failOnParsingToken(t *testing.T) context.Context {
	ctx := failOnNoToken(t)
	// Here we add a token which is incomplete
	token := jwt.New(jwt.GetSigningMethod("RS256"))
	ctx = goajwt.WithJWT(ctx, token)
	return ctx
}

func validToken(t *testing.T, identityID string, identityUsername string) context.Context {
	ctx := failOnNoToken(t)
	// Here we add a token that is perfectly valid
	token, err := testtoken.GenerateTokenObject(identityID, identityUsername, testtoken.PrivateKey())
	require.Nilf(t, err, "could not generate token: %v", errors.WithStack(err))

	ctx = goajwt.WithJWT(ctx, token)
	return ctx
}

func TestExtractUserInfo(t *testing.T) {
	resource.Require(t, resource.UnitTest)

	userID := uuid.NewV4()
	username := "testuser"

	_, err := InitializeSentryClient(nil,
		WithUser(func(ctx context.Context) (*raven.User, error) {
			m, err := token.ReadManagerFromContext(ctx)
			if err != nil {
				return nil, err
			}

			q := *m
			token := goajwt.ContextJWT(ctx)
			if token == nil {
				return nil, fmt.Errorf("no token found in context")
			}
			t, err := q.ParseToken(ctx, token.Raw)
			if err != nil {
				return nil, err
			}

			return &raven.User{
				Username: t.Username,
				Email:    t.Email,
				ID:       t.Subject,
			}, nil
		}))
	require.NoError(t, err)

	tests := []struct {
		name    string
		ctx     context.Context
		want    *raven.User
		wantErr bool
	}{
		{
			name:    "Given some random context",
			ctx:     context.Background(),
			wantErr: true,
		},
		{
			name:    "fail on no token",
			ctx:     failOnNoToken(t),
			wantErr: true,
		},
		{
			name:    "fail on parsing token",
			ctx:     failOnParsingToken(t),
			wantErr: true,
		},
		{
			name:    "pass on parsing token",
			ctx:     validToken(t, userID.String(), username),
			wantErr: false,
			want: &raven.User{
				Username: username,
				ID:       userID.String(),
				Email:    username + "@email.com",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Sentry().userInfo(tt.ctx)
			if tt.wantErr {
				require.Error(t, err)
				// if above assertion passes we don't need to continue
				// to check if objects match
				return
			}
			require.NoError(t, err)
			require.Equalf(t, tt.want, got, "extractUserInfo() = %v, want %v", got, tt.want)
		})
	}
}

func TestDSN(t *testing.T) {
	// Set default DSN via env var
	defaultProject := uuid.NewV4()
	dsn := fmt.Sprintf("https://%s:%s@test.io/%s", uuid.NewV4(), uuid.NewV4(), defaultProject)
	old := os.Getenv("SENTRY_DSN")
	os.Setenv("SENTRY_DSN", dsn)
	defer os.Setenv("SENTRY_DSN", old)

	// Init DSN explicitly
	project := uuid.NewV4()
	dsn = fmt.Sprintf("https://%s:%s@test.io/%s", uuid.NewV4(), uuid.NewV4(), project)
	_, err := InitializeSentryClient(&dsn)
	require.NoError(t, err)

	// The env var is not used. Explicitly set DSN is used instead.
	assert.Equal(t, fmt.Sprintf("https://test.io/api/%s/store/", project), Sentry().c.URL())

	// Init the default DSN
	_, err = InitializeSentryClient(nil)
	require.NoError(t, err)

	// The DSN from the env var is used
	assert.Equal(t, fmt.Sprintf("https://test.io/api/%s/store/", defaultProject), Sentry().c.URL())
}
