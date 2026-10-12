package deck

import (
	"fmt"
	"github.com/golang-jwt/jwt/v5"
)

type Identity struct {
	UserID    string
	AccountID string
	Email     string
	Plan      string
}

func tokenClaims(token string) (jwt.MapClaims, error) {
	claims := jwt.MapClaims{}
	// 与 CLI 相同，这里只解码 HTTPS OAuth 返回的 JWT 元数据，不用于验证签名。
	_, _, err := jwt.NewParser().ParseUnverified(token, claims)
	if err != nil {
		return nil, fmt.Errorf("认证令牌格式无效: %w", err)
	}
	return claims, nil
}

func identityFromToken(token string) (Identity, error) {
	claims, err := tokenClaims(token)
	if err != nil {
		return Identity{}, err
	}
	auth, _ := claims["https://api.openai.com/auth"].(map[string]interface{})
	value := func(m map[string]interface{}, key string) string { v, _ := m[key].(string); return v }
	i := Identity{UserID: value(auth, "chatgpt_user_id"), AccountID: value(auth, "chatgpt_account_id"), Email: value(claims, "email"), Plan: value(auth, "chatgpt_plan_type")}
	if i.UserID == "" {
		i.UserID = value(auth, "user_id")
	}
	if i.UserID == "" {
		i.UserID, _ = claims.GetSubject()
	}
	if i.Email == "" {
		profile, _ := claims["https://api.openai.com/profile"].(map[string]interface{})
		i.Email = value(profile, "email")
	}
	if i.UserID == "" || i.AccountID == "" {
		return i, fmt.Errorf("认证令牌缺少用户或工作区身份")
	}
	if fedramp, _ := auth["chatgpt_account_is_fedramp"].(bool); fedramp {
		return i, fmt.Errorf("当前版本不支持 FedRAMP 工作区")
	}
	return i, nil
}

func identityMatches(a Account, i Identity) bool {
	return a.UserID == i.UserID && a.AccountID == i.AccountID
}
