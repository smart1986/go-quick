package http

import (
	"github.com/gin-gonic/gin"
	"github.com/smart1986/go-quick/logger"
	"strings"
)

var AllRoutes []*Route
var Energy *gin.Engine

type Route struct {
	Path      string
	Method    string
	Handler   gin.HandlerFunc
	Protected bool
}
type IAuthMiddleware interface {
	AuthMiddleware() gin.HandlerFunc
}

func Init(addr string, authMiddleware IAuthMiddleware, block bool) {

	Energy = gin.Default()
	protected := Energy.Group("/")
	protected.Use(authMiddleware.AuthMiddleware())
	for _, route := range AllRoutes {
		if route.Protected {
			registerRoutes(protected, route)
		} else {
			registerRoutes(nil, route)
		}
	}
	if block {
		logger.Info("HTTP server started at", addr)
		err := Energy.Run(addr)
		if err != nil {
			return
		}
	} else {
		go func() {
			logger.Info("HTTP server started at", addr)
			err := Energy.Run(addr)
			if err != nil {
				return
			}
		}()
	}
}
func InitNoAuth(addr string, block bool) {

	Energy = gin.Default()
	for _, route := range AllRoutes {
		registerRoutes(nil, route)
	}
	if block {
		logger.Info("HTTP server started at", addr)
		err := Energy.Run(addr)
		if err != nil {
			return
		}
	} else {
		go func() {
			logger.Info("HTTP server started at", addr)
			err := Energy.Run(addr)
			if err != nil {
				return
			}
		}()
	}
}

func RegisterRoute(path string, method string, handler gin.HandlerFunc, protected bool) {
	route := &Route{
		Path:      path,
		Method:    method,
		Handler:   handler,
		Protected: protected,
	}
	AllRoutes = append(AllRoutes, route)
}

// RegisterRoutes 注册路由到 Gin Engine
func registerRoutes(group *gin.RouterGroup, route *Route) {
	method := strings.ToUpper(route.Method)
	if group == nil {
		switch method {
		case "GET":
			Energy.GET(route.Path, route.Handler)
		case "POST":
			Energy.POST(route.Path, route.Handler)
		case "PUT":
			Energy.PUT(route.Path, route.Handler)
		case "DELETE":
			Energy.DELETE(route.Path, route.Handler)
		default:
			logger.Error("Invalid HTTP method", "method", route.Method)
		}
	} else {
		switch method {
		case "GET":
			group.GET(route.Path, route.Handler)
		case "POST":
			group.POST(route.Path, route.Handler)
		case "PUT":
			group.PUT(route.Path, route.Handler)
		case "DELETE":
			group.DELETE(route.Path, route.Handler)
		default:
			logger.Error("Invalid HTTP method", "method", route.Method)
		}
	}
	logger.Info("Route registered ,method:", route.Method, ",path:", route.Path)

}
