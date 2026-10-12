package deck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Backend struct {
	Client  *http.Client
	BaseURL string
}

type usageWindow struct {
	UsedPercent *float64 `json:"used_percent"`
	Seconds     int64    `json:"limit_window_seconds"`
	ResetAt     *int64   `json:"reset_at"`
	ResetAfter  *int64   `json:"reset_after_seconds"`
}
type usageResponse struct {
	Plan      string `json:"plan_type"`
	AccountID string `json:"account_id"`
	UserID    string `json:"user_id"`
	RateLimit *struct {
		Primary   *usageWindow `json:"primary_window"`
		Secondary *usageWindow `json:"secondary_window"`
	} `json:"rate_limit"`
	ResetCredits *struct {
		AvailableCount *int `json:"available_count"`
	} `json:"rate_limit_reset_credits"`
}

func (b Backend) request(ctx context.Context, a Account, method, path string, body []byte, result interface{}) error {
	_, err := requestJSON(ctx, b.Client, method, b.BaseURL+path, "application/json", body, map[string]string{"Authorization": "Bearer " + a.Tokens.AccessToken, "ChatGPT-Account-Id": a.AccountID}, result)
	return err
}

func (b Backend) Usage(ctx context.Context, a Account) (*Usage, error) {
	var payload usageResponse
	if err := b.request(ctx, a, http.MethodGet, "/wham/usage", nil, &payload); err != nil {
		return nil, err
	}
	if payload.Plan == "" && payload.RateLimit == nil && payload.ResetCredits == nil {
		return nil, fmt.Errorf("用量响应缺少套餐、窗口和重置卡信息")
	}
	if (payload.AccountID != "" && payload.AccountID != a.AccountID) || (payload.UserID != "" && payload.UserID != a.UserID) {
		return nil, fmt.Errorf("用量响应账号身份不匹配")
	}
	u := &Usage{Plan: payload.Plan, QueriedAt: time.Now().UTC()}
	if payload.ResetCredits != nil {
		u.ResetCredits = payload.ResetCredits.AvailableCount
		if u.ResetCredits != nil && *u.ResetCredits < 0 {
			return nil, fmt.Errorf("重置卡数量无效")
		}
	}
	if payload.RateLimit != nil {
		for _, window := range []*usageWindow{payload.RateLimit.Primary, payload.RateLimit.Secondary} {
			if window == nil {
				continue
			}
			if window.Seconds != 18000 && window.Seconds != 604800 {
				continue
			}
			if window.UsedPercent == nil {
				return nil, fmt.Errorf("用量窗口缺少已用比例")
			}
			if *window.UsedPercent < 0 || *window.UsedPercent > 100 {
				return nil, fmt.Errorf("用量窗口比例无效")
			}
			w := &Window{RemainingPercent: 100 - *window.UsedPercent}
			if window.ResetAt != nil {
				at := time.Unix(*window.ResetAt, 0).UTC()
				w.ResetAt = &at
			} else if window.ResetAfter != nil {
				at := u.QueriedAt.Add(time.Duration(*window.ResetAfter) * time.Second)
				w.ResetAt = &at
			}
			if window.Seconds == 18000 {
				u.FiveHour = w
			} else {
				u.Weekly = w
			}
		}
	}
	return u, nil
}

func (b Backend) Credits(ctx context.Context, a Account) (*Credits, error) {
	var payload struct {
		AvailableCount *int     `json:"available_count"`
		Credits        []Credit `json:"credits"`
	}
	if err := b.request(ctx, a, http.MethodGet, "/wham/rate-limit-reset-credits", nil, &payload); err != nil {
		return nil, err
	}
	if payload.AvailableCount == nil || *payload.AvailableCount < 0 {
		return nil, fmt.Errorf("重置卡响应缺少有效数量")
	}
	return &Credits{AvailableCount: *payload.AvailableCount, Credits: payload.Credits, QueriedAt: time.Now().UTC()}, nil
}

type ConsumeResult struct {
	Code         string `json:"code"`
	WindowsReset int    `json:"windows_reset"`
	RefreshError string `json:"refresh_error"`
}

func (b Backend) Consume(ctx context.Context, a Account, requestID, creditID string) (ConsumeResult, error) {
	var result ConsumeResult
	payload := struct {
		RequestID string `json:"redeem_request_id"`
		CreditID  string `json:"credit_id,omitempty"`
	}{requestID, creditID}
	body, err := json.Marshal(payload)
	if err != nil {
		return result, err
	}
	if err = b.request(ctx, a, http.MethodPost, "/wham/rate-limit-reset-credits/consume", body, &result); err != nil {
		return result, err
	}
	switch result.Code {
	case "reset", "already_redeemed", "nothing_to_reset", "no_credit":
		return result, nil
	default:
		return result, fmt.Errorf("未知的重置卡消费结果: %q", result.Code)
	}
}
