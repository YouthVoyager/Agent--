package auth

import "context"

type identityContextKey struct{}

// Identity 表示通过鉴权解析出的调用方身份。
type Identity struct {
	KeyID    string
	UserID   string
	TenantID string
	Scopes   []string
}

// WithIdentity 将调用方身份写入请求上下文。
func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityContextKey{}, cloneIdentity(identity))
}

// IdentityFromContext 从请求上下文读取调用方身份。
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityContextKey{}).(Identity)
	if !ok {
		return Identity{}, false
	}
	return cloneIdentity(identity), true
}

func cloneIdentity(identity Identity) Identity {
	identity.Scopes = append([]string(nil), identity.Scopes...)
	return identity
}
