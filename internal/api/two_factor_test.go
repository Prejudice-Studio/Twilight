package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prejudice-studio/twilight/internal/migration"
	"github.com/prejudice-studio/twilight/internal/security"
	"github.com/prejudice-studio/twilight/internal/store"
)

const factorTestPassword = "Factor-test-password-42!"

func TestTwoFactorPasswordChangeRetainsFactorAndRotatesSession(t *testing.T) {
	app := newTestApp(t)
	u, _, codes, _ := factorEnroll(t, app)
	request := factorChallenge(t, app, u)
	r := factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": request, "code": codes[0], "recovery": true}, nil)
	factorData(t, r)
	oldCookies := r.Result().Cookies()
	newPassword := "Changed-factor-password-42!"
	r = factorRequest(app, "POST", "/api/v2/settings/password/change", map[string]any{"old_password": factorTestPassword, "new_password": newPassword}, oldCookies)
	factorData(t, r)
	cookies := r.Result().Cookies()
	if rr := factorRequest(app, "GET", "/api/v2/settings/two-factor", nil, oldCookies); rr.Code != 401 {
		t.Fatal("password change retained old session")
	}
	if data := factorData(t, factorRequest(app, "GET", "/api/v2/settings/two-factor", nil, cookies)); data["enabled"] != true {
		t.Fatal("password change lost factor")
	}
	r = factorRequest(app, "PUT", "/api/v2/settings/password/generate", map[string]any{"old_password": newPassword}, cookies)
	generated := factorData(t, r)["new_password"].(string)
	if data := factorData(t, factorRequest(app, "POST", "/api/v2/auth/login", map[string]any{"username": u.Username, "password": generated}, nil)); data["two_factor_required"] != true {
		t.Fatal("generated password bypassed factor")
	}
}

func TestTwoFactorCancelRevokesAlreadyIssuedSession(t *testing.T) {
	app := newTestApp(t)
	u, _, codes, _ := factorEnroll(t, app)
	token := factorChallenge(t, app, u)
	r := factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": token, "code": codes[0], "recovery": true}, nil)
	factorData(t, r)
	cookies := r.Result().Cookies()
	factorData(t, factorRequest(app, "DELETE", "/api/v2/auth/two-factor", map[string]any{"request": token}, nil))
	if r = factorRequest(app, "GET", "/api/v2/settings/two-factor", nil, cookies); r.Code != 401 {
		t.Fatal("cancelled in-flight login retained a session", r.Code)
	}
}

func factorRequest(app *App, method, path string, body any, cookies []*http.Cookie) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	return doJSONWithHeaders(app, method, path, string(b), cookies, map[string]string{"X-Twilight-Device": "factor-browser-device-123", "User-Agent": "Firefox"})
}
func factorData(t *testing.T, r *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if r.Code != 200 {
		t.Fatalf("response %d %s", r.Code, r.Body.String())
	}
	var e struct{ Data map[string]any }
	if err := json.Unmarshal(r.Body.Bytes(), &e); err != nil {
		t.Fatal(err)
	}
	return e.Data
}
func factorEnroll(t *testing.T, app *App) (store.User, string, []string, []*http.Cookie) {
	t.Helper()
	t.Setenv("TWILIGHT_TWO_FACTOR_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	return factorEnrollWithCurrentKey(t, app)
}

func factorEnrollWithCurrentKey(t *testing.T, app *App) (store.User, string, []string, []*http.Cookie) {
	t.Helper()
	app.cfg().TwoFactorEnrollmentEnabled = true
	password, _ := security.HashPassword(factorTestPassword)
	u, err := app.store().CreateUser(store.User{Username: "factor-user", PasswordHash: password, Active: true, TelegramID: 789})
	if err != nil {
		t.Fatal(err)
	}
	r := factorRequest(app, "POST", "/api/v2/auth/login", map[string]any{"username": u.Username, "password": factorTestPassword}, nil)
	factorData(t, r)
	cookies := r.Result().Cookies()
	setup := factorData(t, factorRequest(app, "POST", "/api/v2/settings/two-factor/setup", map[string]any{"password": factorTestPassword}, cookies))
	secret := setup["secret"].(string)
	code, _ := security.TOTPCode(secret, time.Now())
	data := factorData(t, factorRequest(app, "POST", "/api/v2/settings/two-factor/enable", map[string]any{"request": setup["request"], "code": code}, cookies))
	var codes []string
	for _, c := range data["recovery_codes"].([]any) {
		codes = append(codes, c.(string))
	}
	return u, secret, codes, cookies
}
func factorChallenge(t *testing.T, app *App, user store.User) string {
	r := factorRequest(app, "POST", "/api/v2/auth/login", map[string]any{"username": user.Username, "password": factorTestPassword}, nil)
	d := factorData(t, r)
	if d["two_factor_required"] != true || d["token"] != nil || d["user"] != nil || len(r.Result().Cookies()) != 0 {
		t.Fatal("primary proof issued a session")
	}
	return d["request"].(string)
}

func TestTwoFactorPasswordRecoveryAndSwitch(t *testing.T) {
	app := newTestApp(t)
	u, secret, codes, oldCookies := factorEnroll(t, app)
	if rr := factorRequest(app, "GET", "/api/v2/settings/two-factor", nil, oldCookies); rr.Code != 401 {
		t.Fatal("old session survived enable", rr.Code)
	}
	app.cfg().TwoFactorEnrollmentEnabled = false
	token := factorChallenge(t, app, u)
	factor, err := app.store().TwoFactorAccount(context.Background(), u.UID)
	if err != nil {
		t.Fatal(err)
	}
	// Reuse the actual enrollment step even if the wall clock crossed a boundary.
	code, _ := security.TOTPCode(secret, time.Unix(factor.LastStep*30, 0))
	if rr := factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": token, "code": code}, nil); rr.Code != 401 {
		t.Fatal("enrollment code replay accepted", rr.Code)
	}
	r := factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": token, "code": codes[0], "recovery": true}, nil)
	d := factorData(t, r)
	if d["user"] == nil || len(r.Result().Cookies()) == 0 {
		t.Fatal("missing session")
	}
	cookies := r.Result().Cookies()
	if rr := factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": token, "code": codes[1], "recovery": true}, nil); rr.Code != 409 {
		t.Fatal("challenge reused")
	}
	token = factorChallenge(t, app, u)
	if rr := factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": token, "code": codes[0], "recovery": true}, nil); rr.Code != 401 {
		t.Fatal("recovery code reused")
	}
	status := factorData(t, factorRequest(app, "GET", "/api/v2/settings/two-factor", nil, cookies))
	if status["enabled"] != true || status["recovery_remaining"] != float64(9) {
		t.Fatal(status)
	}
	factorData(t, factorRequest(app, "DELETE", "/api/v2/settings/two-factor", map[string]any{"password": factorTestPassword, "code": codes[1], "recovery": true}, cookies))
	if rr := factorRequest(app, "GET", "/api/v2/settings/two-factor", nil, cookies); rr.Code != 401 {
		t.Fatal("disabled factor retained old session")
	}
	r = factorRequest(app, "POST", "/api/v2/auth/login", map[string]any{"username": u.Username, "password": factorTestPassword}, nil)
	if factorData(t, r)["user"] == nil {
		t.Fatal("login after disable")
	}
}

func TestTwoFactorConcurrentAndInvalidation(t *testing.T) {
	app := newTestApp(t)
	u, _, codes, _ := factorEnroll(t, app)
	token := factorChallenge(t, app, u)
	peer := reopenTestStore(t)
	key, _ := security.TwoFactorKey()
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			st := app.store()
			if i%2 == 0 {
				st = peer
			}
			if _, err := st.ConsumeTwoFactorLogin(context.Background(), security.TwoFactorDigest("request", token), "factor-browser-device-123", codes[0], true, key); err == nil {
				successes.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("concurrent consumes", successes.Load())
	}
	for _, field := range []string{"password", "active", "telegram", "role", "rebind"} {
		t.Run(field, func(t *testing.T) {
			token := factorChallenge(t, app, u)
			_, err := peer.UpdateUser(u.UID, func(v *store.User) error {
				switch field {
				case "password":
					v.PasswordHash = "changed"
				case "active":
					v.Active = false
				case "telegram":
					v.TelegramID = 999
				case "role":
					v.Role++
				case "rebind":
					v.RebindingInProgress = true
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			_, err = peer.UpdateUser(u.UID, func(v *store.User) error {
				v.PasswordHash = u.PasswordHash
				v.Active = u.Active
				v.TelegramID = u.TelegramID
				v.Role = u.Role
				v.RebindingInProgress = u.RebindingInProgress
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if rr := factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": token, "code": codes[1], "recovery": true}, nil); rr.Code != 409 {
				t.Fatalf("%s reverted security change revived request: %d", field, rr.Code)
			}
		})
	}
}

func TestTwoFactorTelegramCannotBypass(t *testing.T) {
	app := newTestApp(t)
	newFakeTelegramServer(t, app)
	app.cfg().TelegramLoginEnabled = true
	_, _, codes, _ := factorEnroll(t, app)
	l := issueLoginQR(t, app)
	approveLoginQR(t, app, l)
	r := loginQRRequest(app, "POST", "/api/v2/auth/telegram/requests/"+l.ID+"/consume", l.Secret)
	d := factorData(t, r)
	if d["two_factor_required"] != true || len(r.Result().Cookies()) != 0 {
		t.Fatal("QR bypassed factor")
	}
	body, _ := json.Marshal(map[string]any{"request": d["request"], "code": codes[0], "recovery": true})
	r = doJSONWithHeaders(app, "POST", "/api/v2/auth/two-factor", string(body), nil, map[string]string{"X-Twilight-Device": "qr-browser-device-123456", "User-Agent": "Firefox QR Test"})
	if factorData(t, r)["user"] == nil || len(r.Result().Cookies()) == 0 {
		t.Fatal("approved QR plus second factor did not create a session")
	}
	l = issueLoginQR(t, app)
	approveLoginQR(t, app, l)
	d = factorData(t, loginQRRequest(app, "POST", "/api/v2/auth/telegram/requests/"+l.ID+"/consume", l.Secret))
	body, _ = json.Marshal(map[string]any{"request": d["request"], "code": codes[1], "recovery": true})
	app.cfg().TelegramLoginEnabled = false
	r = doJSONWithHeaders(app, "POST", "/api/v2/auth/two-factor", string(body), nil, map[string]string{"X-Twilight-Device": "qr-browser-device-123456", "User-Agent": "Firefox QR Test"})
	if r.Code != 409 {
		t.Fatal("disabled QR login accepted", r.Code)
	}
}

func TestTwoFactorSetupSessionOwnershipAndClosedEnrollment(t *testing.T) {
	app := newTestApp(t)
	u, _, _, _ := factorEnroll(t, app)
	if err := app.store().ResetTwoFactor(context.Background(), u.UID); err != nil {
		t.Fatal(err)
	}
	login := func() []*http.Cookie {
		r := factorRequest(app, "POST", "/api/v2/auth/login", map[string]any{"username": u.Username, "password": factorTestPassword}, nil)
		factorData(t, r)
		return r.Result().Cookies()
	}
	owner, another := login(), login()
	if r := factorRequest(app, "POST", "/api/v2/settings/two-factor/setup", map[string]any{"password": "wrong"}, owner); r.Code != 403 {
		t.Fatal("setup accepted wrong password", r.Code)
	}
	setup := factorData(t, factorRequest(app, "POST", "/api/v2/settings/two-factor/setup", map[string]any{"password": factorTestPassword}, owner))
	code, _ := security.TOTPCode(setup["secret"].(string), time.Now())
	body := map[string]any{"request": setup["request"], "code": code}
	if r := factorRequest(app, "POST", "/api/v2/settings/two-factor/enable", body, another); r.Code != 409 {
		t.Fatal("another session completed setup", r.Code)
	}
	app.cfg().TwoFactorEnrollmentEnabled = false
	if r := factorRequest(app, "POST", "/api/v2/settings/two-factor/enable", body, owner); r.Code != 403 {
		t.Fatal("closed enrollment enabled factor", r.Code)
	}
	app.cfg().TwoFactorEnrollmentEnabled = true
	if r := factorRequest(app, "POST", "/api/v2/settings/two-factor/enable", body, owner); r.Code != 409 {
		t.Fatal("reopening enrollment revived old setup", r.Code)
	}
	setup = factorData(t, factorRequest(app, "POST", "/api/v2/settings/two-factor/setup", map[string]any{"password": factorTestPassword}, owner))
	code, _ = security.TOTPCode(setup["secret"].(string), time.Now())
	factorData(t, factorRequest(app, "POST", "/api/v2/settings/two-factor/enable", map[string]any{"request": setup["request"], "code": code}, owner))
}

func TestTwoFactorAttemptsCancelExpiryAndMissingKey(t *testing.T) {
	app := newTestApp(t)
	u, _, codes, _ := factorEnroll(t, app)
	token := factorChallenge(t, app, u)
	for i := 0; i < 5; i++ {
		r := factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": token, "code": "bad"}, nil)
		if r.Code != 401 {
			t.Fatal(r.Code)
		}
	}
	if r := factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": token, "code": codes[0], "recovery": true}, nil); r.Code != 409 {
		t.Fatal("attempt budget bypass")
	}
	token = factorChallenge(t, app, u)
	factorData(t, factorRequest(app, "DELETE", "/api/v2/auth/two-factor", map[string]any{"request": token}, nil))
	if r := factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": token, "code": codes[0], "recovery": true}, nil); r.Code != 409 {
		t.Fatal("cancelled accepted")
	}
	token = factorChallenge(t, app, u)
	_, err := app.store().DB().Exec(`UPDATE twilight_two_factor_requests SET expires_at=1`)
	if err != nil {
		t.Fatal(err)
	}
	if r := factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": token, "code": codes[0], "recovery": true}, nil); r.Code != 409 {
		t.Fatal("expired accepted")
	}
	token = factorChallenge(t, app, u)
	t.Setenv("TWILIGHT_TWO_FACTOR_KEY", "")
	if r := factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": token, "code": codes[0], "recovery": true}, nil); r.Code != 503 {
		t.Fatal("missing key allowed login", r.Code)
	}
}

func TestTwoFactorSnapshotRejectsWrongKeyAndRevokesCache(t *testing.T) {
	app := newTestApp(t)
	u, _, codes, _ := factorEnroll(t, app)
	token := factorChallenge(t, app, u)
	r := factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": token, "code": codes[0], "recovery": true}, nil)
	factorData(t, r)
	cookie := findCookie(r.Result().Cookies(), "twilight_session")
	peer := newSessionStoreWithDB(time.Hour, nil, reopenTestStore(t))
	if _, ok := peer.Get(context.Background(), cookie.Value); !ok {
		t.Fatal("peer did not see login")
	}
	backup, err := app.store().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TWILIGHT_TWO_FACTOR_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32))))
	if err = app.store().LoadSnapshot(backup); err == nil {
		t.Fatal("wrong key restored")
	}
	if _, ok := peer.Get(context.Background(), cookie.Value); !ok {
		t.Fatal("failed restore modified database")
	}
	t.Setenv("TWILIGHT_TWO_FACTOR_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	if err = app.store().LoadSnapshot(backup); err != nil {
		t.Fatal(err)
	}
	if _, ok := peer.Get(context.Background(), cookie.Value); ok {
		t.Fatal("peer cache survived restore")
	}
	factor, err := app.store().TwoFactorAccount(context.Background(), u.UID)
	if err != nil || factor.EnabledAt == 0 || len(factor.Recovery) != 9 {
		t.Fatal("factor lost in backup", fmt.Sprint(err))
	}
}

func TestTwoFactorTOTPRefreshRotationAndMigration(t *testing.T) {
	app := newTestApp(t)
	u, secret, codes, _ := factorEnroll(t, app)
	token := factorChallenge(t, app, u)
	code, _ := security.TOTPCode(secret, time.Now().Add(30*time.Second))
	r := factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": token, "code": code}, nil)
	factorData(t, r)
	cookies := r.Result().Cookies()
	r = factorRequest(app, "POST", "/api/v2/auth/refresh", nil, cookies)
	factorData(t, r)
	cookies = r.Result().Cookies()
	rotated := factorData(t, factorRequest(app, "POST", "/api/v2/settings/two-factor/recovery-codes", map[string]any{"password": factorTestPassword, "code": codes[0], "recovery": true}, cookies))
	newCode := rotated["recovery_codes"].([]any)[0].(string)
	if rr := factorRequest(app, "GET", "/api/v2/settings/two-factor", nil, cookies); rr.Code != 401 {
		t.Fatal("rotation kept session")
	}
	token = factorChallenge(t, app, u)
	if rr := factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": token, "code": codes[1], "recovery": true}, nil); rr.Code != 401 {
		t.Fatal("old recovery list accepted")
	}
	factorData(t, factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": token, "code": newCode, "recovery": true}, nil))
	files, err := app.store().ExportMigrationFiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	archive := migration.Archive{Files: map[string][]byte{}}
	for _, f := range files {
		archive.Files[f.Path] = f.Data
	}
	if err = app.store().ResetTwoFactor(context.Background(), u.UID); err != nil {
		t.Fatal(err)
	}
	if _, err = app.store().ImportMigrationArchive(context.Background(), archive); err != nil {
		t.Fatal(err)
	}
	factor, err := app.store().TwoFactorAccount(context.Background(), u.UID)
	if err != nil || factor.EnabledAt == 0 || len(factor.Recovery) != 9 {
		t.Fatal("migration lost factor", err)
	}
}

func TestTwoFactorDeviceSessionFailureAndV1(t *testing.T) {
	app := newTestApp(t)
	u, _, codes, _ := factorEnroll(t, app)
	r := factorRequest(app, "POST", "/api/v1/auth/login", map[string]any{"username": u.Username, "password": factorTestPassword}, nil)
	d := factorData(t, r)
	if d["two_factor_required"] != true {
		t.Fatal("V1 bypass")
	}
	token := d["request"].(string)
	wrong, _ := json.Marshal(map[string]any{"request": token, "code": codes[0], "recovery": true})
	if rr := doJSONWithHeaders(app, "POST", "/api/v2/auth/two-factor", string(wrong), nil, map[string]string{"X-Twilight-Device": "another-browser-12345"}); rr.Code != 409 {
		t.Fatal("foreign device")
	}
	if _, err := app.store().DB().Exec(`ALTER TABLE twilight_sessions ADD CONSTRAINT factor_deny_session CHECK(uid<0)`); err != nil {
		t.Fatal(err)
	}
	r = factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": token, "code": codes[0], "recovery": true}, nil)
	if r.Code != 500 || len(r.Result().Cookies()) != 0 {
		t.Fatal("session failure", r.Code)
	}
	if _, err := app.store().DB().Exec(`ALTER TABLE twilight_sessions DROP CONSTRAINT factor_deny_session`); err != nil {
		t.Fatal(err)
	}
	if r = factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": token, "code": codes[1], "recovery": true}, nil); r.Code != 409 {
		t.Fatal("failed session reused proof")
	}
	// A subsequent device block and unblock must not resurrect a request.
	token = factorChallenge(t, app, u)
	_, err := app.store().DB().Exec(`UPDATE twilight_state SET state=jsonb_set(state,ARRAY['devices',$1,'is_blocked'],'true'), version=version+1 WHERE id=1`, strconv.FormatInt(u.UID, 36)+":factor-browser-device-123")
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.store().DB().Exec(`UPDATE twilight_state SET state=jsonb_set(state,ARRAY['devices',$1,'is_blocked'],'false'), version=version+1 WHERE id=1`, strconv.FormatInt(u.UID, 36)+":factor-browser-device-123")
	if err != nil {
		t.Fatal(err)
	}
	if r = factorRequest(app, "POST", "/api/v2/auth/two-factor", map[string]any{"request": token, "code": codes[1], "recovery": true}, nil); r.Code != 409 {
		t.Fatal("unblock resurrected request", r.Code)
	}
}
