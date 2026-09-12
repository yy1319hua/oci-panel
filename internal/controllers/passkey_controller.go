package controllers

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"github.com/adiecho/oci-panel/internal/config"
	"github.com/adiecho/oci-panel/internal/database"
	"github.com/adiecho/oci-panel/internal/middleware"
	"github.com/adiecho/oci-panel/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

const (
	PasskeyEnabledKey    = "passkey_enabled"
	PasskeyCredentialKey = "passkey_credential"
)

type PasskeyController struct {
	cfg      *config.Config
	webAuthn *webauthn.WebAuthn
	sessions sync.Map
}

type AdminUser struct {
	id          []byte
	name        string
	displayName string
	credentials []webauthn.Credential
}

func (u *AdminUser) WebAuthnID() []byte {
	return u.id
}

func (u *AdminUser) WebAuthnName() string {
	return u.name
}

func (u *AdminUser) WebAuthnDisplayName() string {
	return u.displayName
}

func (u *AdminUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

func NewPasskeyController(cfg *config.Config) *PasskeyController {
	rpID := cfg.Passkey.RPID
	if rpID == "" {
		rpID = "localhost"
	}
	rpOrigins := cfg.Passkey.RPOrigins
	if len(rpOrigins) == 0 {
		rpOrigins = []string{"http://localhost:8999"}
	}

	wconfig := &webauthn.Config{
		RPDisplayName: "OCI Panel",
		RPID:          rpID,
		RPOrigins:     rpOrigins,
	}

	webAuthn, err := webauthn.New(wconfig)
	if err != nil {
		panic(err)
	}

	return &PasskeyController{
		cfg:      cfg,
		webAuthn: webAuthn,
	}
}

func (pc *PasskeyController) getAdminUser() *AdminUser {
	user := &AdminUser{
		id:          []byte(pc.cfg.Web.Account),
		name:        pc.cfg.Web.Account,
		displayName: "Admin",
		credentials: []webauthn.Credential{},
	}

	db := database.GetDB()
	var credSetting models.SysSetting
	if err := db.Where("key = ?", PasskeyCredentialKey).First(&credSetting).Error; err == nil && credSetting.Value != "" {
		credBytes, err := base64.StdEncoding.DecodeString(credSetting.Value)
		if err == nil {
			var cred webauthn.Credential
			if json.Unmarshal(credBytes, &cred) == nil {
				user.credentials = []webauthn.Credential{cred}
			}
		}
	}

	return user
}

type PasskeyStatusResponse struct {
	Enabled bool `json:"enabled"`
}

func (pc *PasskeyController) GetStatus(c *gin.Context) {
	db := database.GetDB()
	var setting models.SysSetting
	enabled := false
	if err := db.Where("key = ?", PasskeyEnabledKey).First(&setting).Error; err == nil {
		enabled = setting.Value == "true"
	}
	c.JSON(http.StatusOK, models.SuccessResponse(PasskeyStatusResponse{Enabled: enabled}, "success"))
}

func (pc *PasskeyController) BeginRegistration(c *gin.Context) {
	user := pc.getAdminUser()

	options, session, err := pc.webAuthn.BeginRegistration(user,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationPreferred,
		}),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to begin registration"))
		return
	}

	sessionData, _ := json.Marshal(session)
	pc.sessions.Store("reg_"+pc.cfg.Web.Account, sessionData)

	c.JSON(http.StatusOK, models.SuccessResponse(options, "success"))
}

type FinishRegistrationRequest struct {
	Credential json.RawMessage `json:"credential"`
}

func (pc *PasskeyController) FinishRegistration(c *gin.Context) {
	var req FinishRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	sessionData, ok := pc.sessions.Load("reg_" + pc.cfg.Web.Account)
	if !ok {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "No registration session found"))
		return
	}
	pc.sessions.Delete("reg_" + pc.cfg.Web.Account)

	var session webauthn.SessionData
	if err := json.Unmarshal(sessionData.([]byte), &session); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Invalid session data"))
		return
	}

	credentialData, err := protocol.ParseCredentialCreationResponse(
		&http.Request{Body: io.NopCloser(newJSONBody(req.Credential))},
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "Failed to parse credential: "+err.Error()))
		return
	}

	user := pc.getAdminUser()
	credential, err := pc.webAuthn.CreateCredential(user, session, credentialData)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "Failed to create credential: "+err.Error()))
		return
	}

	credBytes, _ := json.Marshal(credential)
	credBase64 := base64.StdEncoding.EncodeToString(credBytes)

	// upsert：保持原有「失败静默」行为（原代码亦未检查写入错误）
	_ = database.UpsertSysSetting(PasskeyCredentialKey, credBase64)
	_ = database.UpsertSysSetting(PasskeyEnabledKey, "true")

	c.JSON(http.StatusOK, models.SuccessResponse(nil, "Passkey registered successfully"))
}

func (pc *PasskeyController) BeginLogin(c *gin.Context) {
	user := pc.getAdminUser()

	if len(user.credentials) == 0 {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "No passkey registered"))
		return
	}

	options, session, err := pc.webAuthn.BeginLogin(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to begin login"))
		return
	}

	sessionData, _ := json.Marshal(session)
	pc.sessions.Store("login_"+pc.cfg.Web.Account, sessionData)

	c.JSON(http.StatusOK, models.SuccessResponse(options, "success"))
}

type FinishLoginRequest struct {
	Credential json.RawMessage `json:"credential"`
}

func (pc *PasskeyController) FinishLogin(c *gin.Context) {
	var req FinishLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, err.Error()))
		return
	}

	sessionData, ok := pc.sessions.Load("login_" + pc.cfg.Web.Account)
	if !ok {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "No login session found"))
		return
	}
	pc.sessions.Delete("login_" + pc.cfg.Web.Account)

	var session webauthn.SessionData
	if err := json.Unmarshal(sessionData.([]byte), &session); err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Invalid session data"))
		return
	}

	credentialData, err := protocol.ParseCredentialRequestResponse(
		&http.Request{Body: newJSONBody(req.Credential)},
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse(400, "Failed to parse credential"))
		return
	}

	user := pc.getAdminUser()
	_, err = pc.webAuthn.ValidateLogin(user, session, credentialData)
	if err != nil {
		c.JSON(http.StatusUnauthorized, models.ErrorResponse(401, "Invalid passkey"))
		return
	}

	token, err := middleware.GenerateToken(pc.cfg.Web.Account)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse(500, "Failed to generate token"))
		return
	}

	c.JSON(http.StatusOK, models.SuccessResponse(LoginResponse{
		Token:    token,
		Username: pc.cfg.Web.Account,
		NeedMFA:  false,
	}, "Passkey login successful"))
}

func (pc *PasskeyController) Disable(c *gin.Context) {
	db := database.GetDB()
	db.Model(&models.SysSetting{}).Where("key = ?", PasskeyEnabledKey).Update("value", "false")
	db.Model(&models.SysSetting{}).Where("key = ?", PasskeyCredentialKey).Update("value", "")
	c.JSON(http.StatusOK, models.SuccessResponse(nil, "Passkey disabled successfully"))
}

type jsonBody struct {
	data []byte
	pos  int
}

func newJSONBody(data json.RawMessage) *jsonBody {
	return &jsonBody{data: data}
}

func (j *jsonBody) Read(p []byte) (n int, err error) {
	if j.pos >= len(j.data) {
		return 0, io.EOF
	}
	n = copy(p, j.data[j.pos:])
	j.pos += n
	return n, nil
}

func (j *jsonBody) Close() error {
	return nil
}
