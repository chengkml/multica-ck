package handler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/auth"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type WxBindRequest struct {
	Email  string `json:"email"`
	Code   string `json:"code"`
	WxCode string `json:"wx_code"`
}

type WxLoginRequest struct {
	Code string `json:"code"`
}

type WxLoginResponse struct {
	Token string       `json:"token"`
	User  UserResponse `json:"user"`
	Bound bool         `json:"bound"`
}

func (h *Handler) WxBind(w http.ResponseWriter, r *http.Request) {
	var req WxBindRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	code := strings.TrimSpace(req.Code)
	wxCode := strings.TrimSpace(req.WxCode)

	if email == "" || code == "" {
		writeError(w, http.StatusBadRequest, "email and code are required")
		return
	}
	if wxCode == "" {
		writeError(w, http.StatusBadRequest, "wx_code is required")
		return
	}

	dbCode, err := h.Queries.GetLatestVerificationCode(r.Context(), email)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired code")
		return
	}

	if !isDevVerificationCode(code) && subtle.ConstantTimeCompare([]byte(code), []byte(dbCode.Code)) != 1 {
		_ = h.Queries.IncrementVerificationCodeAttempts(r.Context(), dbCode.ID)
		writeError(w, http.StatusBadRequest, "invalid or expired code")
		return
	}

	if err := h.Queries.MarkVerificationCodeUsed(r.Context(), dbCode.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to verify code")
		return
	}

	wxSession, err := h.WxService.Code2Session(wxCode)
	if err != nil {
		slog.Warn("wx_bind: code2session failed", "error", err)
		writeError(w, http.StatusBadRequest, "invalid wx_code")
		return
	}

	openidHash := auth.HashToken(wxSession.Openid)

	user, isNew, err := h.findOrCreateUser(r.Context(), email)
	if err != nil {
		var signupErr SignupError
		if errors.As(err, &signupErr) {
			writeError(w, http.StatusForbidden, signupErr.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	if isNew {
		h.Analytics.Capture(analytics.Signup(uuidToString(user.ID), user.Email, ""))
	}

	var unionidHash pgtype.Text
	if wxSession.Unionid != "" {
		unionidHash = pgtype.Text{String: auth.HashToken(wxSession.Unionid), Valid: true}
	}

	_, err = h.Queries.CreateMiniprogramToken(r.Context(), db.CreateMiniprogramTokenParams{
		UserID:      user.ID,
		OpenidHash:  openidHash,
		Appid:       h.WxService.Appid(),
		UnionidHash: unionidHash,
	})
	if err != nil {
		if isUniqueViolation(err) {
			existing, lookupErr := h.Queries.GetMiniprogramTokenByOpenidHash(r.Context(), openidHash)
			if lookupErr == nil && uuidToString(existing.UserID) == uuidToString(user.ID) {
				tokenString, jwtErr := h.issueJWT(user)
				if jwtErr != nil {
					writeError(w, http.StatusInternalServerError, "failed to generate token")
					return
				}
				writeJSON(w, http.StatusOK, WxLoginResponse{
					Token: tokenString,
					User:  userToResponse(user),
					Bound: true,
				})
				return
			}
			writeError(w, http.StatusConflict, "this wechat account is already bound to another user")
			return
		}
		slog.Error("wx_bind: failed to create miniprogram token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to bind wechat account")
		return
	}

	tokenString, err := h.issueJWT(user)
	if err != nil {
		slog.Warn("wx_bind: failed to issue JWT", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	slog.Info("wx_bind: wechat account bound", "user_id", uuidToString(user.ID), "email", email)
	writeJSON(w, http.StatusOK, WxLoginResponse{
		Token: tokenString,
		User:  userToResponse(user),
		Bound: true,
	})
}

func (h *Handler) WxLogin(w http.ResponseWriter, r *http.Request) {
	var req WxLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	wxCode := strings.TrimSpace(req.Code)
	if wxCode == "" {
		writeError(w, http.StatusBadRequest, "code is required")
		return
	}

	wxSession, err := h.WxService.Code2Session(wxCode)
	if err != nil {
		slog.Warn("wx_login: code2session failed", "error", err)
		writeError(w, http.StatusBadRequest, "invalid code")
		return
	}

	openidHash := auth.HashToken(wxSession.Openid)

	if h.MiniprogramTokenCache != nil {
		if userID, ok := h.MiniprogramTokenCache.Get(r.Context(), openidHash); ok {
			user, err := h.Queries.GetUser(r.Context(), parseUUID(userID))
			if err != nil {
				writeError(w, http.StatusUnauthorized, "user not found")
				return
			}

			tokenString, err := h.issueJWT(user)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to generate token")
				return
			}

			writeJSON(w, http.StatusOK, WxLoginResponse{
				Token: tokenString,
				User:  userToResponse(user),
				Bound: true,
			})
			return
		}
	}

	mpToken, err := h.Queries.GetMiniprogramTokenByOpenidHash(r.Context(), openidHash)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusUnauthorized, "wechat account not bound")
			return
		}
		slog.Warn("wx_login: failed to lookup miniprogram token", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to lookup binding")
		return
	}

	user, err := h.Queries.GetUser(r.Context(), mpToken.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}

	var expiresAt time.Time
	if mpToken.ExpiresAt.Valid {
		expiresAt = mpToken.ExpiresAt.Time
	}
	if h.MiniprogramTokenCache != nil {
		h.MiniprogramTokenCache.Set(r.Context(), openidHash, uuidToString(user.ID), auth.TTLForExpiry(time.Now(), expiresAt))
	}

	go h.Queries.UpdateMiniprogramTokenLastUsed(context.Background(), mpToken.ID)

	tokenString, err := h.issueJWT(user)
	if err != nil {
		slog.Warn("wx_login: failed to issue JWT", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	slog.Info("wx_login: user logged in via wechat", "user_id", uuidToString(user.ID))
	writeJSON(w, http.StatusOK, WxLoginResponse{
		Token: tokenString,
		User:  userToResponse(user),
		Bound: true,
	})
}

func (h *Handler) ListMiniprogramTokens(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	tokens, err := h.Queries.ListMiniprogramTokensByUser(r.Context(), parseUUID(userID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tokens")
		return
	}

	type MiniprogramTokenResponse struct {
		ID         string  `json:"id"`
		Appid      string  `json:"appid"`
		ExpiresAt  *string `json:"expires_at"`
		LastUsedAt *string `json:"last_used_at"`
		CreatedAt  string  `json:"created_at"`
	}

	resp := make([]MiniprogramTokenResponse, len(tokens))
	for i, t := range tokens {
		resp[i] = MiniprogramTokenResponse{
			ID:         uuidToString(t.ID),
			Appid:      t.Appid,
			ExpiresAt:  timestampToPtr(t.ExpiresAt),
			LastUsedAt: timestampToPtr(t.LastUsedAt),
			CreatedAt:  timestampToString(t.CreatedAt),
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) RevokeMiniprogramToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	id := chi.URLParam(r, "id")
	idUUID, ok := parseUUIDOrBadRequest(w, id, "token id")
	if !ok {
		return
	}

	openidHash, err := h.Queries.RevokeMiniprogramToken(r.Context(), db.RevokeMiniprogramTokenParams{
		ID:     idUUID,
		UserID: parseUUID(userID),
	})
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to revoke token")
		return
	}

	if h.MiniprogramTokenCache != nil {
		h.MiniprogramTokenCache.Invalidate(r.Context(), openidHash)
	}

	w.WriteHeader(http.StatusNoContent)
}
