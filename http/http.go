package http

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/middleware"
	"github.com/lejianwen/rustdesk-api/v2/http/router"
	"github.com/sirupsen/logrus"
	"net/http"
	"strings"
)

func ApiInit() {
	Run(NewEngine(), global.Config.Gin.ApiAddr)
}

// NewEngine arma el motor con sus middlewares y rutas. Está separado de ApiInit para poder probarlo.
func NewEngine() *gin.Engine {
	gin.SetMode(global.Config.Gin.Mode)
	g := gin.New()

	// Fork adbeels: sin trust-proxy configurado, gin confía en X-Forwarded-For / X-Real-IP de cualquiera,
	// y el límite de intentos de login (que va por IP) se puede evadir o usar contra otra IP.
	// Ahora, si trust-proxy está vacío, no se confía en ningún proxy: la IP del cliente es la de la conexión.
	var pro []string
	for _, p := range strings.Split(global.Config.Gin.TrustProxy, ",") {
		if p = strings.TrimSpace(p); p != "" {
			pro = append(pro, p)
		}
	}
	if err := g.SetTrustedProxies(pro); err != nil {
		panic(err)
	}

	if global.Config.Gin.Mode == gin.ReleaseMode {
		//修改gin Recovery日志 输出为logger的输出点
		if global.Logger != nil {
			gin.DefaultErrorWriter = global.Logger.WriterLevel(logrus.ErrorLevel)
		}
	}
	g.NoRoute(func(c *gin.Context) {
		c.String(http.StatusNotFound, "404 not found")
	})
	g.Use(middleware.SecureHeaders(), middleware.BodyLimit(global.Config.Gin.MaxBodySize),
		middleware.Logger(), middleware.Limiter(), gin.Recovery())
	router.WebInit(g)
	router.Init(g)
	router.ApiInit(g)
	return g
}
