package http

// Pruebas de integración del fork adbeels: cada una cubre un fallo de seguridad corregido en
// v2.7-adbeels.5 (ver MANTENIMIENTO.md) para que no vuelva a entrar sin que el CI lo note.
// Arman el motor real (NewEngine) sobre una base sqlite temporal.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/lib/jwt"
	"github.com/lejianwen/rustdesk-api/v2/lib/lock"
	"github.com/lejianwen/rustdesk-api/v2/model"
	"github.com/lejianwen/rustdesk-api/v2/service"
	"github.com/lejianwen/rustdesk-api/v2/utils"
	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Límite de cuerpo para las pruebas; todas las peticiones legítimas de aquí pesan menos
const maxBody = 2048

var (
	engine     *gin.Engine
	adminToken string
	userToken  string
)

func TestMain(m *testing.M) {
	// resources/ se resuelve desde la raíz del repo
	if err := os.Chdir(".."); err != nil {
		panic(err)
	}
	dir, err := os.MkdirTemp("", "rustdesk-api-test")
	if err != nil {
		panic(err)
	}
	setup(filepath.Join(dir, "test.db"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func setup(dbPath string) {
	global.Config.Lang = "en"
	global.Config.Gin.Mode = gin.TestMode
	global.Config.Gin.ResourcesPath = "resources"
	global.Config.Gin.MaxBodySize = maxBody
	global.Config.App.WebSso = true
	global.Config.App.TokenExpire = 168 * time.Hour
	global.Config.Rustdesk.ApiServer = "http://127.0.0.1:21114"

	global.Logger = logrus.New()
	global.Logger.SetOutput(io.Discard)
	global.InitI18n()
	global.ApiInitValidator()

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{Logger: gormlogger.Discard})
	if err != nil {
		panic(err)
	}
	err = db.AutoMigrate(
		&model.Version{}, &model.User{}, &model.UserToken{}, &model.Tag{}, &model.AddressBook{},
		&model.Peer{}, &model.Group{}, &model.UserThird{}, &model.Oauth{}, &model.LoginLog{},
		&model.AuditConn{}, &model.AuditFile{}, &model.AddressBookCollection{},
		&model.AddressBookCollectionRule{}, &model.ServerCmd{}, &model.DeviceGroup{},
	)
	if err != nil {
		panic(err)
	}
	global.DB = db
	global.Jwt = jwt.NewJwt("", 0)
	global.Lock = lock.NewLocal()
	service.New(&global.Config, db, global.Logger, global.Jwt, global.Lock)
	global.LoginLimiter = utils.NewLoginLimiter(utils.SecurityPolicy{
		CaptchaThreshold: 3,
		BanThreshold:     10,
		AttemptsWindow:   10 * time.Minute,
		BanDuration:      30 * time.Minute,
	})
	global.LoginLimiter.RegisterProvider(utils.B64StringCaptchaProvider{})

	db.Create(&model.Group{Name: "default", Type: model.GroupTypeDefault})
	yes, no := true, false
	admin := &model.User{Username: "admin", Email: "admin@example.test", Status: model.COMMON_STATUS_ENABLE, IsAdmin: &yes, GroupId: 1}
	user := &model.User{Username: "tecnico", Email: "tecnico@example.test", Status: model.COMMON_STATUS_ENABLE, IsAdmin: &no, GroupId: 1}
	db.Create(admin)
	db.Create(user)
	adminToken = newSession(admin)
	userToken = newSession(user)

	engine = NewEngine()
}

func newSession(u *model.User) string {
	return service.AllService.UserService.Login(u, &model.LoginLog{UserId: u.Id, Client: model.LoginLogClientWebAdmin}).Token
}

type reply struct {
	status int
	body   string
	header http.Header
}

// code es el campo "code" de las respuestas de /api/admin (-1 si la respuesta no lo trae)
func (r reply) code() int {
	var v struct {
		Code *int `json:"code"`
	}
	if json.Unmarshal([]byte(r.body), &v) != nil || v.Code == nil {
		return -1
	}
	return *v.Code
}

func call(method, path, body string, headers map[string]string) reply {
	return callFrom("192.0.2.1:1234", method, path, body, headers)
}

func callFrom(remote, method, path, body string, headers map[string]string) reply {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = remote
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return reply{w.Code, w.Body.String(), w.Header()}
}

func asUser() map[string]string  { return map[string]string{"api-token": userToken} }
func asAdmin() map[string]string { return map[string]string{"api-token": adminToken} }

// Rutas de /api/admin que no piden sesión
var publicAdminRoutes = map[string]bool{
	"POST /api/admin/login":          true,
	"GET /api/admin/captcha":         true,
	"POST /api/admin/logout":         true,
	"GET /api/admin/login-options":   true,
	"POST /api/admin/oidc/auth":      true,
	"GET /api/admin/oidc/auth-query": true,
	"POST /api/admin/user/register":  true,
	"GET /api/admin/config/admin":    true,
}

// Rutas de /api/admin (además de /api/admin/my/*) que puede usar cualquier usuario con sesión.
// Todo lo que no esté aquí ni arriba debe exigir administrador: una ruta nueva hace fallar la
// prueba hasta que alguien decida a cuál de las tres categorías pertenece.
var userAdminRoutes = map[string]bool{
	"GET /api/admin/config/server":      true,
	"GET /api/admin/user/current":       true,
	"POST /api/admin/user/changeCurPwd": true,
	"POST /api/admin/user/myOauth":      true,
	"POST /api/admin/user/groupUsers":   true,
	"POST /api/admin/peer/simpleData":   true,
	"POST /api/admin/oauth/confirm":     true,
	"POST /api/admin/oauth/bind":        true,
	"POST /api/admin/oauth/bindConfirm": true,
	"POST /api/admin/oauth/unbind":      true,
	"GET /api/admin/oauth/info":         true,
}

// Incluye /api/admin/rustdesk/* (comandos al puerto local de hbbs/hbbr), que estaba sin AdminPrivilege.
func TestAdminRoutesAreProtected(t *testing.T) {
	param := regexp.MustCompile(`:[A-Za-z_]+`)
	checked := 0
	for _, r := range engine.Routes() {
		key := r.Method + " " + r.Path
		if !strings.HasPrefix(r.Path, "/api/admin/") || publicAdminRoutes[key] {
			continue
		}
		checked++
		path := param.ReplaceAllString(r.Path, "1")
		body := ""
		if r.Method != http.MethodGet {
			body = "{}"
		}
		if got := call(r.Method, path, body, nil).code(); got != 403 {
			t.Errorf("%s sin sesión: code=%d, se esperaba 403", key, got)
		}
		if userAdminRoutes[key] || strings.HasPrefix(r.Path, "/api/admin/my/") {
			continue
		}
		if got := call(r.Method, path, body, asUser()).code(); got != 403 {
			t.Errorf("%s con un usuario no administrador: code=%d, se esperaba 403", key, got)
		}
	}
	if checked < 50 {
		t.Fatalf("solo se revisaron %d rutas de /api/admin; ¿cambió la forma de registrarlas?", checked)
	}
	// Control: el administrador sí entra
	if c := call("GET", "/api/admin/user/list?page=1&page_size=5", "", asAdmin()).code(); c != 0 {
		t.Errorf("el administrador no pudo listar usuarios: code=%d", c)
	}
}

func TestLogoutInvalidatesToken(t *testing.T) {
	u := service.AllService.UserService.InfoByUsername("tecnico")
	h := map[string]string{"api-token": newSession(u)}
	if c := call("GET", "/api/admin/user/current", "", h).code(); c != 0 {
		t.Fatalf("el token recién creado no sirve: code=%d", c)
	}
	if c := call("POST", "/api/admin/logout", "", h).code(); c != 0 {
		t.Fatalf("logout: code=%d", c)
	}
	if c := call("GET", "/api/admin/user/current", "", h).code(); c != 403 {
		t.Fatalf("el token sigue vigente después de cerrar sesión: code=%d", c)
	}
	// Con un token que no existe o sin token responde igual: no sirve para averiguar tokens
	if c := call("POST", "/api/admin/logout", "", map[string]string{"api-token": "no-existe"}).code(); c != 0 {
		t.Errorf("logout con token desconocido: code=%d", c)
	}
	if c := call("POST", "/api/admin/logout", "", nil).code(); c != 0 {
		t.Errorf("logout sin token: code=%d", c)
	}
}

func TestSessionTokenIsRandom(t *testing.T) {
	u := &model.User{Username: "x"}
	a := service.AllService.UserService.GenerateToken(u)
	b := service.AllService.UserService.GenerateToken(u)
	if a == b || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(a) {
		t.Fatalf("se esperaban 64 hexadecimales distintos en cada llamada: %d y %d caracteres, iguales=%v", len(a), len(b), a == b)
	}
}

// Sin gin.trust-proxy, la IP del cliente es la de la conexión y no la que diga una cabecera.
func TestForwardedHeadersAreNotTrusted(t *testing.T) {
	const attacker = "203.0.113.50:40000"
	fail := `{"username":"nadie","password":"clave-equivocada","id":"1","uuid":"x"}`
	for i := 0; i < 10; i++ {
		callFrom(attacker, "POST", "/api/login", fail, map[string]string{"X-Forwarded-For": "198.51.100.7"})
	}
	if c := callFrom("198.51.100.7:5555", "GET", "/api/version", "", nil).code(); c != 0 {
		t.Errorf("quedó bloqueada la IP que el atacante puso en X-Forwarded-For: code=%d", c)
	}
	spoof := map[string]string{"X-Forwarded-For": "198.51.100.9", "X-Real-IP": "198.51.100.9"}
	if c := callFrom(attacker, "GET", "/api/version", "", spoof).code(); c != 423 {
		t.Errorf("la IP real no quedó bloqueada tras 10 intentos fallidos: code=%d", c)
	}
}

const sysinfoJSON = `{"id":"%s","uuid":"%s","hostname":"%s","username":"u","os":"Windows","cpu":"c","memory":"m","version":"1.5.0"}`

func sysinfo(id, uuid, hostname string) reply {
	return call("POST", "/api/sysinfo", fmt.Sprintf(sysinfoJSON, id, uuid, hostname), nil)
}

// /api/sysinfo no pide sesión: quien no sea el equipo dueño del id (otro uuid) no puede sobrescribirlo.
func TestSysInfoKeepsOwnerUuid(t *testing.T) {
	const id = "111111111"
	if b := sysinfo(id, "uuid-A", "CAJA-01").body; b != "SYSINFO_UPDATED" {
		t.Fatalf("alta: %q", b)
	}
	if b := sysinfo(id, "uuid-B", "SUPLANTADO").body; b != "UUID_MISMATCH" {
		t.Fatalf("con otro uuid se esperaba UUID_MISMATCH: %q", b)
	}
	if p := service.AllService.PeerService.FindById(id); p.Hostname != "CAJA-01" || p.Uuid != "uuid-A" {
		t.Fatalf("el equipo se sobrescribió: hostname=%q uuid=%q", p.Hostname, p.Uuid)
	}
	if b := sysinfo(id, "uuid-A", "CAJA-01-nuevo").body; b != "SYSINFO_UPDATED" {
		t.Fatalf("el equipo dueño no pudo actualizarse: %q", b)
	}
	if p := service.AllService.PeerService.FindById(id); p.Hostname != "CAJA-01-nuevo" {
		t.Fatalf("no se guardó la actualización legítima: %q", p.Hostname)
	}
	if r := call("POST", "/api/sysinfo", `{"id":"222222222","hostname":"x"}`, nil); r.status != http.StatusBadRequest {
		t.Errorf("sin uuid se esperaba 400: %d", r.status)
	}
}

func TestHeartbeatNeedsMatchingUuid(t *testing.T) {
	const id = "333333333"
	sysinfo(id, "uuid-C", "PC")
	beat := func(uuid string) int64 {
		call("POST", "/api/heartbeat", fmt.Sprintf(`{"id":"%s","uuid":"%s","ver":1}`, id, uuid), nil)
		return service.AllService.PeerService.FindById(id).LastOnlineTime
	}
	if ts := beat("uuid-otro"); ts != 0 {
		t.Fatalf("un latido con otro uuid marcó el equipo en línea")
	}
	if ts := beat("uuid-C"); ts == 0 {
		t.Fatalf("el latido del equipo dueño no se registró")
	}
}

// /api/audit/* no pide sesión: solo se guarda lo que reporta el equipo dueño del id.
func TestAuditNeedsKnownReporter(t *testing.T) {
	const id = "444444444"
	sysinfo(id, "uuid-D", "PC")
	count := func(m interface{}) (n int64) {
		global.DB.Model(m).Where("peer_id = ?", id).Count(&n)
		return
	}
	conn := func(peer, uuid string) {
		call("POST", "/api/audit/conn", fmt.Sprintf(`{"action":"new","conn_id":7,"id":"%s","uuid":"%s","ip":"203.0.113.9","session_id":1,"peer":["999999999","x"],"type":0}`, peer, uuid), nil)
	}
	conn(id, "uuid-otro")
	conn("555555555", "uuid-D")
	if n := count(&model.AuditConn{}); n != 0 {
		t.Fatalf("se guardaron %d registros de conexión de un emisor que no es el equipo", n)
	}
	conn(id, "uuid-D")
	if n := count(&model.AuditConn{}); n != 1 {
		t.Fatalf("el registro legítimo no se guardó: %d", n)
	}

	file := func(uuid string) {
		call("POST", "/api/audit/file", fmt.Sprintf(`{"id":"%s","uuid":"%s","peer_id":"999999999","type":0,"path":"/tmp/x","is_file":true,"info":"{\"ip\":\"203.0.113.9\",\"name\":\"x\",\"num\":1}"}`, id, uuid), nil)
	}
	file("uuid-otro")
	if n := count(&model.AuditFile{}); n != 0 {
		t.Fatalf("se guardaron %d registros de archivos de un emisor que no es el equipo", n)
	}
	file("uuid-D")
	if n := count(&model.AuditFile{}); n != 1 {
		t.Fatalf("el registro legítimo de archivos no se guardó: %d", n)
	}
}

// app.web-sso=false debe apagar el flujo webauth, no solo ocultarlo en el cliente.
func TestWebauthSwitch(t *testing.T) {
	start := func() (reply, string) {
		r := call("POST", "/api/oidc/auth", `{"op":"webauth","id":"987654321","uuid":"u","deviceInfo":{"name":"PC","os":"windows","type":"client"}}`, nil)
		var v struct {
			Code string `json:"code"`
		}
		json.Unmarshal([]byte(r.body), &v)
		return r, v.Code
	}
	confirm := func(code string) int {
		return call("POST", "/api/admin/oauth/confirm", `{"code":"`+code+`"}`, asUser()).code()
	}

	r, code := start()
	if r.status != http.StatusOK || code == "" {
		t.Fatalf("con web-sso encendido no se pudo iniciar: %d %s", r.status, r.body)
	}
	if info := call("GET", "/api/admin/oauth/info?code="+code, "", asUser()); !strings.Contains(info.body, `"ip":"192.0.2.1"`) {
		t.Errorf("la consola no recibe la IP de quien inició el flujo: %s", info.body)
	}
	if c := confirm(code); c != 0 {
		t.Fatalf("confirmar: code=%d", c)
	}
	if q := call("GET", "/api/oidc/auth-query?code="+code+"&id=987654321&uuid=u", "", nil); !strings.Contains(q.body, `"access_token"`) {
		t.Fatalf("tras confirmar no se entregó la sesión: %s", q.body)
	}

	_, pending := start()
	global.Config.App.WebSso = false
	defer func() { global.Config.App.WebSso = true }()
	if r, _ := start(); r.status != http.StatusBadRequest {
		t.Errorf("con web-sso apagado el flujo se pudo iniciar: %d %s", r.status, r.body)
	}
	// El cliente RustDesk espera una lista en common-oidc/, también cuando está vacía
	if opts := call("GET", "/api/login-options", "", nil).body; !strings.Contains(opts, "common-oidc/[]") {
		t.Errorf("login-options sin proveedores debe traer una lista vacía: %s", opts)
	}
	if c := confirm(pending); c == 0 {
		t.Errorf("con web-sso apagado se pudo confirmar un código pendiente")
	}
}

func TestBodyLimit(t *testing.T) {
	big := `{"id":"1","uuid":"x","path":"` + strings.Repeat("a", maxBody) + `"}`
	if r := call("POST", "/api/audit/file", big, nil); r.status != http.StatusRequestEntityTooLarge {
		t.Errorf("cuerpo mayor al límite: se esperaba 413, llegó %d", r.status)
	}
	// Sin Content-Length (chunked) el límite se aplica al leer el cuerpo
	req := httptest.NewRequest("POST", "/api/audit/file", io.NopCloser(strings.NewReader(big)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("cuerpo sin Content-Length mayor al límite: se esperaba 400, llegó %d", w.Code)
	}
}

func TestSecurityHeadersAndNoDirectoryListing(t *testing.T) {
	r := call("GET", "/api/version", "", nil)
	want := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Content-Security-Policy": "frame-ancestors 'none'",
	}
	for k, v := range want {
		if got := r.header.Get(k); got != v {
			t.Errorf("cabecera %s: %q, se esperaba %q", k, got, v)
		}
	}
	if r := call("GET", "/upload/", "", nil); strings.Contains(r.body, ".gitkeep") {
		t.Errorf("/upload/ lista su contenido: %s", r.body)
	}
}

// groupUsers lo usa cualquier usuario con sesión para elegir a quién compartir una libreta.
func TestGroupUsersOnlyReturnsWhatIsNeeded(t *testing.T) {
	r := call("POST", "/api/admin/user/groupUsers", "{}", asUser())
	if r.code() != 0 || !strings.Contains(r.body, `"username":"admin"`) {
		t.Fatalf("groupUsers: %s", r.body)
	}
	if strings.Contains(r.body, "example.test") || strings.Contains(r.body, `"is_admin":true`) {
		t.Errorf("groupUsers devuelve correo o quién es administrador: %s", r.body)
	}
}
