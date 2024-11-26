package http

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"github.com/smart1986/go-quick/logger"
	"io"
	"strings"
	"time"
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
	Energy.Use(logRequestParams())
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

func logRequestParams() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 记录请求时间
		start := time.Now()

		// 打印请求信息
		reqBody, _ := c.GetRawData()
		logger.Debugf("Request: %s %s %s\n", c.Request.Method, c.Request.RequestURI, reqBody)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(reqBody))
		// 执行请求处理程序和其他中间件函数
		c.Next()

		// 记录回包内容和处理时间
		end := time.Now()
		latency := end.Sub(start)
		//respBody := string(c.Writer.Body.Bytes())
		logger.Debugf(" Response: %s %s %s \n", c.Request.Method, c.Request.RequestURI, latency)
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
