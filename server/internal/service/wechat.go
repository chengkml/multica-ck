package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type WxSessionResponse struct {
	Openid     string `json:"openid"`
	SessionKey string `json:"session_key"`
	Unionid    string `json:"unionid"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

type WxService struct {
	client *http.Client
}

func NewWxService() *WxService {
	return &WxService{
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *WxService) Appid() string {
	return strings.TrimSpace(os.Getenv("WX_APPID"))
}

func (s *WxService) secret() string {
	return strings.TrimSpace(os.Getenv("WX_SECRET"))
}

func (s *WxService) Code2Session(code string) (*WxSessionResponse, error) {
	appid := s.Appid()
	secret := s.secret()
	if appid == "" || secret == "" {
		return nil, fmt.Errorf("WX_APPID or WX_SECRET not configured")
	}

	u := url.URL{
		Scheme: "https",
		Host:   "api.weixin.qq.com",
		Path:   "/sns/jscode2session",
		RawQuery: url.Values{
			"appid":      {appid},
			"secret":     {secret},
			"js_code":    {code},
			"grant_type": {"authorization_code"},
		}.Encode(),
	}

	resp, err := s.client.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("wx code2session request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("wx code2session read body failed: %w", err)
	}

	var result WxSessionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("wx code2session parse response failed: %w", err)
	}

	if result.ErrCode != 0 {
		slog.Warn("wx code2session error", "errcode", result.ErrCode, "errmsg", result.ErrMsg)
		return nil, fmt.Errorf("wx code2session error: %d %s", result.ErrCode, result.ErrMsg)
	}

	if result.Openid == "" {
		return nil, fmt.Errorf("wx code2session returned empty openid")
	}

	return &result, nil
}
