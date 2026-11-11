# Mantenimiento del fork adbeels/rustdesk-api

Fork de [lejianwen/rustdesk-api](https://github.com/lejianwen/rustdesk-api) (licencia MIT, crédito a lejianwen).
Upstream no tiene commits desde el 2025-09-29 (v2.7) y sus dependencias se quedaron viejas. Este fork las
mantiene al día porque la API va a correr expuesta a Internet y controla los equipos de soporte.

Reglas del fork:
- **No se reintroduce el cliente web** (`webclient`/`webclient2`, retirado por DMCA). La imagen no incluye
  `resources/web` y `app.web-client` está en `0` por omisión.
- Imagen con **etiqueta fija** `vX.Y-adbeels.N` en `ghcr.io/adbeels/rustdesk-api`. Nunca `latest`.
- Sin secretos en el repo.

## Cómo se construye y se publica

| Pieza | Dónde |
|---|---|
| Imagen | `Dockerfile.adbeels`: multietapa, construye la consola y la API; bases fijadas por versión y digest |
| Consola web | Fork [adbeels/rustdesk-api-web](https://github.com/adbeels/rustdesk-api-web), fijado por commit en `WEB_COMMIT` (`npm ci --ignore-scripts`). Hasta `v2.7-adbeels.1` era `lejianwen/rustdesk-api-web` `3998c2a9213f`, el mismo de la v2.7 oficial |
| CI | `.github/workflows/imagen.yml`: `go vet`, pruebas, `govulncheck` (si encuentra algo, no publica) y la imagen. Además corre cada lunes solo la revisión: si sale una vulnerabilidad nueva, falla y GitHub avisa por correo |
| Publicar | `git tag v2.7-adbeels.N && git push origin v2.7-adbeels.N` → `ghcr.io/adbeels/rustdesk-api:v2.7-adbeels.N` |

- En PR y a mano (`workflow_dispatch`) el workflow construye la imagen pero **no la publica**.
- Se quitaron los workflows de upstream (`build.yml`, `build_test.yml`):
  - publicaban `latest` y pedían secretos de Docker Hub;
  - bajaban toolchains de un sitio de terceros (`musl.ljw.red`);
  - su disparador `v*.*.*` también atrapa las etiquetas del fork.
- `go.sum` ahora se versiona. Upstream lo ignoraba y lo regeneraba en cada build con `go mod tidy`.

**Para medir localmente sin Go instalado** (en pos-server, contenedor efímero; revisa antes `srv quien` y la RAM):

```bash
docker run --rm -e GOTOOLCHAIN=auto -v "$PWD":/src -w /src golang:1.27.2-bookworm sh -c 'go install golang.org/x/vuln/cmd/govulncheck@v1.9.0 && /go/bin/govulncheck ./...'
```

`GOTOOLCHAIN=auto` hace que se use la versión de Go que fija `go.mod` (la misma del Dockerfile y del CI). Sin eso,
la imagen `golang` usa la suya y govulncheck mediría la librería estándar de otra versión.

## 2026-10-18 · Línea base y primera actualización (`v2.7-adbeels.1`)

Herramientas: govulncheck v1.9.0, base de vulnerabilidades del 2026-10-08. Contenedor efímero
`golang:1.27.2-bookworm` en pos-server (en la laptop no hay Go ni Docker).

### Antes

Se midió de dos formas:
- **Modo binario**: sobre el `apimain` de la imagen oficial `lejianwen/rustdesk-api:v2.7`, que es lo que corre
  hoy en pruebas. Ese binario va sin símbolos (`-s -w`), así que govulncheck no puede saber qué se llama: cuenta
  todas las vulnerabilidades conocidas de los módulos y de la versión de Go que trae dentro.
- **Modo fuente**: sobre el código de v2.7 con su toolchain original (go1.23.12). Solo cuenta lo que el código
  **sí llama**, según el grafo de llamadas.

| Origen | Go | Vulnerabilidades | Solo importadas o requeridas |
|---|---|---|---|
| Binario de la imagen v2.7 | go1.23.12 | **110** presentes en sus módulos | — |
| Código v2.7 (fuente) | go1.23.12 | **55** alcanzables | 57 |

> Corrección del 2026-11-11: esta tabla decía «110 alcanzables» para el binario. Alcanzables desde el código eran
> 55; las 110 son las que venían dentro del binario, se llamaran o no.

**Por módulo:**

| Módulo | Binario | Fuente |
|---|---|---|
| stdlib (Go 1.23.12, sin soporte desde 2025-08) | 57 | 47 |
| golang.org/x/crypto v0.33.0 | 21 | 0 |
| golang.org/x/net v0.34.0 | 10 | 2 |
| golang.org/x/image v0.13.0 (vía base64Captcha) | 10 | 0 |
| github.com/jackc/pgx/v5 v5.6.0 | 3 | 1 |
| golang.org/x/text v0.22.0 | 2 | 2 |
| github.com/go-jose/go-jose/v4 v4.0.2 | 2 | 1 |
| github.com/sirupsen/logrus v1.8.1 | 1 | 1 |
| github.com/golang-jwt/jwt/v5 v5.2.1 | 1 | 1 |
| github.com/gin-gonic/gin v1.9.0 | 1 | 0 |
| golang.org/x/oauth2 v0.23.0 | 1 | 0 |
| github.com/Azure/go-ntlmssp (vía ldap) | 1 | 0 |

**Las más relevantes para un servidor expuesto** (alcanzables desde el código):
- **net/http**: desincronización HTTP/1 tras CONNECT (GO-2026-6613), agotamiento de memoria por cookies,
  `Range` y trailers HTTP/2 (GO-2025-4012, GO-2026-6609, GO-2026-6603) y CPU por ventanas HTTP/2 (GO-2026-6611).
- **crypto/tls y crypto/x509**: reanudación de sesión inesperada (GO-2026-4337), KeyUpdate sin autenticar
  (GO-2026-4870) y varias de complejidad cuadrática al validar certificados.
- **html/template**: cinco fallos de escape que permiten XSS (GO-2026-4865, -4980, -4982, -6091, -6599).
- **golang-jwt** v5.2.1: asignación excesiva de memoria al leer el encabezado del token (GO-2025-3553).
- **pgx** v5.6.0: inyección SQL con literales `$$` (GO-2026-5004). Solo aplica con PostgreSQL; aquí usamos sqlite.

**Directas desactualizadas** (`go list -m -u`):

| Módulo | Tenía | Última |
|---|---|---|
| github.com/BurntSushi/toml | v1.3.2 | v1.6.0 |
| github.com/coreos/go-oidc/v3 | v3.12.0 | v3.21.0 |
| github.com/gin-gonic/gin | v1.9.0 | v1.12.0 |
| github.com/go-ldap/ldap/v3 | v3.4.10 | v3.4.15 |
| github.com/go-playground/locales | v0.14.1 | v0.14.2 |
| github.com/go-playground/universal-translator | v0.18.1 | v0.18.2 |
| github.com/go-playground/validator/v10 | v10.26.0 | v10.30.5 |
| github.com/go-redis/redis/v8 | v8.11.4 | v8.11.5 |
| github.com/golang-jwt/jwt/v5 | v5.2.1 | v5.3.1 |
| github.com/mojocn/base64Captcha | v1.3.6 | v1.3.8 |
| github.com/nicksnyder/go-i18n/v2 | v2.4.0 | v2.6.1 |
| github.com/sirupsen/logrus | v1.8.1 | v1.10.2 |
| github.com/spf13/cobra | v1.8.1 | v1.10.2 |
| github.com/spf13/viper | v1.9.0 | v1.21.0 |
| github.com/swaggo/gin-swagger | v1.6.0 | v1.6.1 |
| github.com/swaggo/swag | v1.16.3 | v1.16.6 |
| golang.org/x/crypto | v0.33.0 | v0.58.0 |
| golang.org/x/oauth2 | v0.23.0 | v0.37.0 |
| golang.org/x/text | v0.22.0 | v0.43.0 |
| gorm.io/driver/mysql | v1.5.7 | v1.6.0 |
| gorm.io/driver/postgres | v1.6.0 | v1.6.3 |
| gorm.io/driver/sqlite | v1.5.6 | v1.6.0 |
| gorm.io/gorm | v1.25.10 | v1.31.2 |

Al día ya estaban: google/uuid v1.6.0, swaggo/files v1.0.1 y antonfisher/nested-logrus-formatter v1.3.1.
Sin versiones nuevas: fvbock/endless (2017).

### Qué se cambió

- **Go**: `go 1.26.0`, `toolchain go1.27.2`. Las dos versiones con soporte al 2026-10-18 son 1.27.2 y 1.26.9.
- **Dependencias**: `go get -u` de todas las directas y `go mod tidy`; quedaron en las versiones de la columna
  «Última». De paso subieron las indirectas, p. ej. x/net v0.61.0, x/image v0.47.0, pgx v5.11.0,
  go-jose v4.1.4 y mattn/go-sqlite3 v1.14.52.
- **Código**: no hizo falta cambiar nada para compilar. `go vet` con Go ≥ 1.24 marcó 12 `t.Fatalf` con formato
  no constante en las pruebas de `lib/cache`; se cambiaron por `t.Fatal`.
- `conf/config.yaml`: `app.web-client` pasa de `1` a `0`.

### Después

| Origen | Go | Vulnerabilidades alcanzables | Solo importadas o requeridas |
|---|---|---|---|
| Código actualizado (fuente) | go1.27.2 | **0** | 1 |
| Binario nuevo | go1.27.2 | **0** | 1 |

La única que queda es GO-2026-5932: `golang.org/x/crypto/openpgp` está abandonado. No tiene arreglo, el código
no importa ese paquete y solo aparece porque el módulo `x/crypto` está en el grafo.

### Pruebas

- `go build`: ✅. `go vet ./...`: ✅.
- `go test -skip Redis ./...`: ✅ (cache, jwt, lock, service, utils).
  - Las 5 pruebas de Redis fallan **igual que en upstream**: apuntan a un Redis fijo del autor (`192.168.1.168:6379`).
- **Paridad con v2.7**: binario nuevo y binario de la imagen v2.7 sobre una **copia** de la base de pruebas,
  con la misma configuración que el ambiente (sin red, en contenedor efímero). Las 16 sondas dieron la misma
  respuesta en los dos:
  - `/api/version`, `login-options` de cliente y consola, `config/admin`;
  - 401/403 sin sesión;
  - sin `/webclient` ni `/webclient2`;
  - registro cerrado;
  - captcha a partir del 3.er intento fallido.

### Publicación y prueba en el ambiente

- Imagen `ghcr.io/adbeels/rustdesk-api:v2.7-adbeels.1`, digest
  `sha256:a9c11a59c699a3f26c4afefc10145d5e10348ca77cf5cc6146eb33a3ccc42be9`. Pesa ~25 MB comprimida.
  - Corrección del 2026-11-11: aquí decía «la oficial v2.7, 99 MB», y comparaba comprimido con desempaquetado.
    Las dos pesan lo mismo: ~26 MB comprimidas y ~99 MB desempaquetadas.
- 2026-10-18 18:40: desplegada en el ambiente de pruebas (pos-server, `rustdesk-pruebas`) en lugar de la v2.7,
  con respaldo previo.
- Verificado:
  - consola 200 y carga;
  - latidos de los 2 equipos;
  - logs sin errores;
  - un cliente 1.5.0 con sesión de usuario sigue viendo su libreta compartida.

### Pendiente (no se hizo en esta ronda)

- sqlite usa el modo de journal por omisión. En el disco mecánico de pruebas gorm marca a veces «SLOW SQL >= 1s» en el
  `UPDATE peers` del latido. Si se repite en el VPS, evaluar el modo WAL (`_journal_mode=WAL`).

- ~~go-redis v8~~, ~~endless~~, ~~imagen como root~~: hechos en `v2.7-adbeels.3`.
- ~~Fase 2: la consola~~: hecha en `v2.7-adbeels.2`.

## 2026-10-18 · Consola con dependencias al día (`v2.7-adbeels.2`)

- La imagen construye la consola desde el fork **adbeels/rustdesk-api-web**, commit `18e92cd`.
  - Detalle en el `MANTENIMIENTO.md` de ese repo.
  - `npm audit`: **28 vulnerabilidades (2 críticas, 19 altas) → 0**. Antes, 12 estaban en el código que llega al
    navegador, entre ellas `protobufjs` (crítica), que el código usaba sin declararla.
  - vite 8.3.4, vue 3.5.44, pinia 4.1.0, vue-router 5.4.0, axios 1.20.0 y element-plus 2.14.7.
  - Cambios de código mínimos: `defineStore('id', …)` y `minify: 'oxc'`.
- La API no cambió respecto a `v2.7-adbeels.1`.
- Imagen `ghcr.io/adbeels/rustdesk-api:v2.7-adbeels.2`, digest
  `sha256:9f940c7eed09f2cab44c762954fe0c0b478d1cfb2b81f3e882ba714b68fff98b`.
- Prueba en el ambiente (19:15, con respaldo previo):
  - consola 200;
  - login sin errores de JavaScript;
  - el admin recorrió dispositivos, libretas y sus reglas, usuarios y grupos, auditoría y cambio de idioma: todo bien;
  - latidos de los 2 equipos y logs sin errores;
  - el cliente 1.5.0 con sesión de `soporte` sigue viendo su libreta compartida.

## 2026-10-18 · Ronda 2 (`v2.7-adbeels.3`)

**API**
- `github.com/go-redis/redis/v8` → **`github.com/redis/go-redis/v9` v9.23.0**. Para Get/Set la API es la misma.
  Las 5 pruebas de Redis pasan contra un Redis 7.4 efímero, apuntando a él en lugar del servidor del autor.
- **Fuera `fvbock/endless`** (abandonado desde 2017). `http/run.go` usa un `http.Server` estándar en todas las
  plataformas:
  - `ReadHeaderTimeout` de 10 s, contra conexiones lentas tipo slowloris; antes no había ningún timeout;
  - `IdleTimeout` de 120 s y `MaxHeaderBytes` de 1 MiB;
  - apagado ordenado con SIGINT/SIGTERM: `docker stop` tarda 0.1 s en lugar de esperar a que Docker lo mate.
- `/api/admin/address_book/shareByWebClient` solo existe con `web-client: 1`, igual que `/api/shared-peer`. Antes
  cualquier usuario con sesión podía crear enlaces del cliente web aunque estuviera apagado.
- **Imagen sin root**: corre como `rustdesk` (1000:1000).
  - El volumen `/app/data` y su `rustdeskapi.db` deben ser de 1000:1000. Para un servidor nuevo:
    `install -d -o 1000 -g 1000 /ruta/datos/api`.
  - Con datos creados por una versión anterior (root), antes de cambiar: `chown -R 1000:1000 /ruta/datos/api`.
  - **Monta solo la llave pública** en el contenedor de la API (`…/id_ed25519.pub:/keys/id_ed25519.pub:ro`), no la
    carpeta de hbbs: ahí también está la llave privada, y la API no la necesita.
- `go vet` (Linux y Windows), pruebas y govulncheck: OK, 0 alcanzables.
- Mismas 16 sondas que en la `.1` sobre una copia de la base, con la misma respuesta. Además,
  `shareByWebClient` → 404.

**Consola** (adbeels/rustdesk-api-web `cb0eb51`; detalle en su `MANTENIMIENTO.md`)
- Fuera todo lo del cliente web: utilidades, diálogo y botones para compartir.
  - Ya no se copia el token del admin a `wc-option:local:access_token` en `localStorage`. Antes quedaba ahí
    incluso después de cerrar sesión.
  - Fuera `protobufjs`, `long` y `ts-proto`.
- `@element-plus/icons` → `@element-plus/icons-vue`.
- `vite.config.js` → `vite.config.mjs`.
- `npm audit`: 0.

**Publicación y prueba (`v2.7-adbeels.3`)**
- Imagen `ghcr.io/adbeels/rustdesk-api:v2.7-adbeels.3`, digest
  `sha256:28a042271007c8efe096b43723cd9daf77ca55eb5a00eef009027f20bd2fa6dd`.
- Primero se probó la imagen sobre una copia de los datos, sin red:
  - corre como uid 1000;
  - `/app/data` y `runtime` escribibles, `conf` de solo lectura;
  - solo la `.pub` montada.
- 19:54: desplegada en el ambiente de pruebas, con respaldo previo. En el compose se cambió también el montaje de
  la llave, para montar solo la `.pub`.
- Verificado:
  - latido escrito por el proceso sin root;
  - logs limpios;
  - consola con iconos y llave, sin botones del cliente web;
  - libreta compartida y conexión desde el cliente 1.5.0.

## 2026-10-18 · Ronda 3: limpieza del cliente web (`v2.7-adbeels.4`)

**API**
- **Fuera todo el cliente web.**
  - Rutas: `/api/shared-peer`, `/api/server-config`, `/api/server-config-v2`, `/webclient-config/index.js`,
    `/webclient/` y `/webclient2/`.
  - También su controlador y sus respuestas, `resources/web` (cliente web v1, 16 MB, 140 archivos) y
    `.gitattributes`.
- **Fuera los «registros compartidos»** (`/api/admin/share_record/*`, `/api/admin/my/share_record/*` y
  `/api/admin/address_book/shareByWebClient`): controladores, servicio, formulario, modelo y rutas.
  - Guardaban la **contraseña de los equipos compartidos en texto plano** (`share_records.password`) y solo
    servían a `/api/shared-peer`.
  - La tabla `share_records` ya no se migra: **un servidor nuevo no la crea**. En una base existente se queda
    como está, sin borrarse sola. En pruebas tenía 0 filas.
  - Si en una base vieja tuviera filas: `sqlite3 rustdeskapi.db "DROP TABLE share_records;"`, con respaldo previo.
- Fuera `/api/admin/config/app`, que solo devolvía `web_client`, y los campos de config `app.web-client`,
  `rustdesk.webclient-magic-queryonline` y `rustdesk.ws-host`.
  - `RUSTDESK_API_APP_WEB_CLIENT` ya no hace nada. Se puede dejar en el compose para poder regresar a una imagen
    anterior.
- Se quedan a propósito:
  - la etiqueta `webclient` en los registros de login, que es un valor histórico;
  - el mensaje de OIDC «Authorization in progress», que es genérico.
- Docs de swagger regenerados con swag v1.16.6 (antes los generaba upstream con otra versión); gofmt.
- `go vet` (Linux y Windows), pruebas y govulncheck: OK, 0 alcanzables.
- Sondas sobre una copia de la base con `RUSTDESK_API_APP_WEB_CLIENT=1` forzado:
  - las 10 rutas quitadas dan 404;
  - las 7 de siempre responden igual.

**Consola** (adbeels/rustdesk-api-web `117aef2`)
- Fuera las vistas, la API y las rutas de registros compartidos, `getAppConfig`, imports sin uso y 19 claves de
  i18n que solo usaba el cliente web.
- Los iconos se registran de forma explícita: 18 en lugar de 293. El chunk de iconos baja de 240 a 102 kB.
- Element Plus completo sigue siendo el chunk más grande: ~780 kB, ~230 kB gzip, y se cachea. Se subió
  `chunkSizeWarningLimit` a 800 y se dejó escrita la razón. Bajarlo exigiría importarlo por componente
  (unplugin-vue-components), y para una consola interna no lo vale.
- `npm audit`: 0.

**Publicación y prueba (`v2.7-adbeels.4`)**
- Imagen `ghcr.io/adbeels/rustdesk-api:v2.7-adbeels.4`, digest
  `sha256:5d66398b86d0446e2000757ec0566ff92c81615a8e4948bf3abdd8e1b9dcbda4`.
- 21:48: desplegada en el ambiente de pruebas, con respaldo previo. Solo cambió la imagen.
- Verificado:
  - consola 200 y login sin errores de JavaScript;
  - rutas quitadas en 404;
  - más de una hora arriba sin reinicios ni errores, con latidos de los 2 equipos;
  - el admin recorrió el menú (todos los iconos), dispositivos, libretas y reglas, usuarios, auditoría e idioma,
    sin «Registros compartidos»;
  - libreta compartida y conexión desde el cliente 1.5.0.

## 2026-11-11 · Ronda 4: seguridad (`v2.7-adbeels.5`)

Sale de una revisión de seguridad de todo el proyecto hecha el 2026-11-10 sobre `v2.7-adbeels.4`: lectura del
código (sesiones, permisos, entradas sin autenticación) y comprobación de cada punto en un contenedor efímero.
Los fallos venían del código heredado, no de las dependencias. Todos quedaron cubiertos por pruebas automáticas
(`http/security_test.go`) que corren en el CI.

### Qué se corrigió

1. **Comandos al servidor solo para administradores.** `/api/admin/rustdesk/{sendCmd,cmdList,cmdCreate,cmdDelete}`
   no pedían `AdminPrivilege`: cualquier usuario con sesión podía mandar comandos al puerto de administración
   local de hbbs y hbbr (`relay-servers`, `must-login`, `ip-blocker`, `blacklist-add`, `total-bandwidth`…).
2. **El límite de intentos ya no confía en cabeceras.** Con `gin.trust-proxy` vacío, gin aceptaba
   `X-Forwarded-For` y `X-Real-IP` de cualquiera. Como el bloqueo por intentos fallidos va por IP, se podía evadir
   cambiando la cabecera, o bloquear la IP de otro. Ahora, sin `trust-proxy`, la IP del cliente es la de la conexión.
3. **Token de sesión aleatorio.** Era `md5(usuario + hora)`, y `/api/sysinfo_ver` publica la hora de arranque.
   Ahora son 32 bytes de `crypto/rand` (64 hexadecimales). Los tokens ya emitidos siguen valiendo hasta caducar.
4. **Cerrar sesión en la consola invalida el token.** `POST /api/admin/logout` está fuera del middleware de
   sesión y su controlador buscaba el token en el contexto, donde nunca estaba. Ahora lo toma de la cabecera y lo
   borra. La consola, además, no llamaba a esa ruta (arreglado en adbeels/rustdesk-api-web).
5. **Inventario y auditoría: solo reporta el equipo dueño del ID.** `/api/sysinfo`, `/api/heartbeat` y
   `/api/audit/*` no piden sesión (así funciona el cliente). Antes, conociendo un ID, cualquiera podía
   sobrescribir los datos de ese equipo, incluido su uuid, e inventar o alterar registros de auditoría.
   - Ahora, si el ID ya tiene un uuid registrado, solo se aceptan datos que traigan ese mismo uuid.
   - `sysinfo` con otro uuid responde `UUID_MISMATCH` y no cambia nada; latidos y auditorías se ignoran.
   - Las auditorías de un ID que la API no conoce también se ignoran.
   - Cada rechazo deja un aviso en el log con el ID y la IP.
   - Comprobado con datos reales del cliente 1.5.0: latidos, auditorías y logins traen el mismo uuid del alta.
6. **Límite de tamaño de las peticiones.** No había: 100 MB sin autenticar subían la memoria del proceso de 69 a
   311 MB. Ahora el máximo es `gin.max-body-size` (4 MiB por omisión): con `Content-Length` mayor responde 413 sin
   leer, y sin `Content-Length` corta al pasar el límite. El servidor tiene además `ReadTimeout` de 30 s y
   `WriteTimeout` de 60 s.
7. **La autorización web se puede apagar de verdad.** Es el inicio de sesión del cliente confirmando desde la
   consola: quien inicia el flujo recibe la sesión de quien pulse «Confirmar» en el enlace.
   - `app.web-sso: false` solo lo ocultaba en el cliente; ahora el flujo no se puede iniciar ni confirmar.
   - La confirmación desde la consola ya no vale para códigos de otros proveedores.
   - Se guarda la IP de quien inicia el flujo y la consola la muestra antes de confirmar.
   - Con `web-sso` apagado y sin proveedores, `login-options` devolvía `common-oidc/null` y el cliente fallaba
     al leerlo; ahora devuelve una lista vacía (mismo arreglo que alonginwind/rustdesk-api `077bd11`).

**Menores, en el mismo paquete:**
- `/api/admin/user/groupUsers` lo usa cualquier usuario para elegir a quién compartir una libreta, y devolvía la
  ficha completa de todos (correo, quién es administrador, notas). Ahora solo `id`, `username` y `group_id`.
- `/_admin/` y `/upload/` ya no listan el contenido de sus carpetas.
- Cabeceras `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`,
  `Content-Security-Policy: frame-ancestors 'none'` y `Referrer-Policy: same-origin` en todas las respuestas.

### Qué cambia para quien despliega

- **Detrás de un proxy inverso hay que poner `RUSTDESK_API_GIN_TRUST_PROXY`** con la IP del proxy. Si no, todos
  los clientes llegan con la IP del proxy y diez contraseñas fallidas bloquean a todos.
- **Un equipo nuevo que reutiliza el ID de otro** no puede actualizar sus datos hasta que se borre el equipo
  anterior en la consola. El aviso `SysInfo rejected: uuid mismatch` en el log lo delata.
- Peticiones de más de 4 MiB se rechazan. Se ajusta con `RUSTDESK_API_GIN_MAX_BODY_SIZE` (en bytes).
- Si los técnicos entran con usuario y contraseña, conviene `RUSTDESK_API_APP_WEB_SSO=false`.

### Mantenimiento

- **Go 1.27.2 → 1.27.4** (`toolchain go1.27.4`). Con la base del 2026-11-11, govulncheck marcaba en la `.4` dos
  vulnerabilidades alcanzables de la librería estándar (GO-2026-6859 y GO-2026-6809) y dos sin llamar en
  `x/net`. Después: **0 alcanzables**; queda el aviso informativo de `x/crypto/openpgp`, que no tiene arreglo.
- Dependencias: `x/net` v0.62.0, `x/text` v0.44.0, `x/oauth2` v0.38.0, validator v10.30.6, go-i18n v2.7.0.
- Imágenes base: `golang:1.27.4-alpine3.24`, `alpine:3.24.4`, `node:24.22.2-alpine3.24`, por digest.
- Consola: commit `71d1e34` (cierre de sesión contra el servidor, IP en la confirmación, `npm audit` 0).
- **Revisión semanal.** Se dijo en octubre que Dependabot abriría PR cada semana. No ocurre: GitHub no activa
  Dependabot en forks, y en tres semanas nadie avisó de las vulnerabilidades nuevas. Ahora `imagen.yml` corre
  cada lunes `go vet`, las pruebas y govulncheck.

### Pruebas

- `go vet` (Linux y Windows) y `go test -skip Redis ./...`: OK.
- `http/security_test.go`, 11 pruebas sobre el motor real con una base temporal:
  - recorre **todas** las rutas de `/api/admin` sin sesión y con un usuario no administrador; una ruta nueva sin
    clasificar hace fallar la prueba;
  - logout, token, cabeceras de proxy, uuid en sysinfo, latido y auditoría, autorización web, límite de cuerpo,
    cabeceras, listado de carpetas y `groupUsers`.
  - Se comprobó que detectan lo que dicen: al quitar a propósito cuatro de los arreglos, fallaron justo esas.
- Laboratorio: las siete comprobaciones que daban positivo en la `.4` dieron negativo, y lo legítimo siguió
  funcionando (cliente con su uuid, sesión de consola y de cliente, proxy de confianza configurado).

### Publicación y prueba (`v2.7-adbeels.5`)

- Imagen `ghcr.io/adbeels/rustdesk-api:v2.7-adbeels.5`, digest
  `sha256:2a8b57aa1b9f89dda2ea10d158fe2759699611ab1861079701feae0e0fb322b4`. Binario con Go 1.27.4.
- Antes de desplegar se repitió el laboratorio completo sobre la imagen publicada (sin red, base vacía):
  - los siete puntos corregidos, cerrados;
  - 100 MB con y sin `Content-Length`: 413 y 400 en milisegundos, con la memoria sin moverse;
  - con `trust-proxy=127.0.0.1` (caso nginx) el bloqueo se aplica por cliente, como debe;
  - `max-body-size` y `web-sso=false` funcionan por variable de entorno.
- 2026-11-11 00:24: desplegada en el ambiente de pruebas, con respaldo previo. En el compose se agregó un tope de
  memoria y de tamaño de logs al contenedor, y la base quedó con permisos 600.
- Verificado en el ambiente:
  - el cliente RustDesk 1.5.0 que corre en el servidor mandó su latido y la API lo aceptó con la validación de
    uuid, sin avisos de rechazo;
  - consola: vistas, comandos del servidor (como administrador) y cierre de sesión. Tras pulsar «Cerrar sesión»
    en el navegador, el token dejó de existir en la base;
  - log sin errores.
- Queda un paquete de Alpine con versión más nueva en su repositorio (`zlib`). El binario es estático y no lo usa.

### Pendiente (no se hizo en esta ronda)

- Los tokens se guardan en claro en la base, y `/api/admin/user_token/list` se los devuelve al administrador.
- Contraseñas válidas desde 4 caracteres y sin segundo factor (upstream tampoco lo tiene).
- El login del cliente (`/api/login`) no puede pedir captcha; solo lo protege el bloqueo por IP.
- Dar de alta equipos sigue sin pedir autenticación (es el protocolo del cliente): contra una avalancha de altas
  falsas, el límite de peticiones va en el proxy inverso.
- sqlite sin WAL (ver la nota de la ronda 1).
