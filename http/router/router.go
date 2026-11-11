package router

import (
	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/controller/web"
)

func WebInit(g *gin.Engine) {
	i := &web.Index{}
	g.GET("/", i.Index)
	// gin.Dir(..., false): sirve los archivos pero no lista el contenido de las carpetas
	g.StaticFS("/_admin", gin.Dir(global.Config.Gin.ResourcesPath+"/admin", false))
}
