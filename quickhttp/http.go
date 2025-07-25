package quickhttp

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"github.com/smart1986/go-quick/logger"
	"go.uber.org/zap"
	"io"
	"strings"
	"time"
)

type HttpServer struct {
	Name      string
	AllRoutes []*Route
	Energy    *gin.Engine
}

type Route struct {
	Path      string
	Method    string
	Handler   gin.HandlerFunc
	Protected bool
}
type IMiddleware interface {
	Middleware() gin.HandlerFunc
}

func (httpServer *HttpServer) Init(addr string, auth IMiddleware, block bool, middleware ...IMiddleware) {

	httpServer.Energy = gin.Default()
	var middlewareList []gin.HandlerFunc
	for _, m := range middleware {
		middlewareList = append(middlewareList, m.Middleware())
	}
	// 打印请求信息
	middlewareList = append(middlewareList, logRequestParams())

	httpServer.Energy.Use(middlewareList...)
	if auth != nil {
		protected := httpServer.Energy.Group("/")
		protected.Use(auth.Middleware())

		for _, route := range httpServer.AllRoutes {
			if route.Protected {
				registerRoutes(protected, route, httpServer)
			} else {
				registerRoutes(nil, route, httpServer)
			}
		}
	} else {
		for _, route := range httpServer.AllRoutes {
			registerRoutes(nil, route, httpServer)
		}
	}

	if block {
		logger.Info(httpServer.Name, " HTTP server started at", addr)
		err := httpServer.Energy.Run(addr)
		if err != nil {
			return
		}
	} else {
		go func() {
			logger.Info(httpServer.Name, " HTTP server started at", addr)
			err := httpServer.Energy.Run(addr)
			if err != nil {
				return
			}
		}()
	}
}

type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w responseBodyWriter) Write(b []byte) (int, error) {
	w.body.Write(b) // 缓存响应内容
	return w.ResponseWriter.Write(b)
}

func logRequestParams() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.RequestURI == "/health" {
			c.Next()
			return
		}

		if logger.DefaultLogger.LogLevel == zap.DebugLevel {
			// 记录请求时间
			start := time.Now()

			reqBody, _ := c.GetRawData()
			logger.Debugf("Request: %s %s %s\n", c.Request.Method, c.Request.RequestURI, string(reqBody))
			c.Request.Body = io.NopCloser(bytes.NewBuffer(reqBody))
			// 执行请求处理程序和其他中间件函数

			// 替换默认的 ResponseWriter
			writer := &responseBodyWriter{
				ResponseWriter: c.Writer,
				body:           bytes.NewBufferString(""),
			}
			c.Writer = writer
			c.Next()

			// 记录回包内容和处理时间
			end := time.Now()
			latency := end.Sub(start)
			logger.Debugf(" Response: %s %s %s %s \n", c.Request.Method, c.Request.RequestURI, latency, writer.body.String())
		} else {
			c.Next()
		}

	}
}

func (httpServer *HttpServer) InitNoAuth(addr string, block bool, middleware ...IMiddleware) {

	httpServer.Energy = gin.Default()
	var middlewareList []gin.HandlerFunc
	for _, m := range middleware {
		middlewareList = append(middlewareList, m.Middleware())
	}
	middlewareList = append(middlewareList, logRequestParams())
	httpServer.Energy.Use(middlewareList...)
	for _, route := range httpServer.AllRoutes {
		registerRoutes(nil, route, httpServer)
	}
	if block {
		logger.Info(httpServer.Name, " HTTP server started at", addr)
		err := httpServer.Energy.Run(addr)
		if err != nil {
			return
		}
	} else {
		go func() {
			logger.Info(httpServer.Name, " HTTP server started at", addr)
			err := httpServer.Energy.Run(addr)
			if err != nil {
				return
			}
		}()
	}
}

func (httpServer *HttpServer) RegisterRoute(path string, method string, handler gin.HandlerFunc, protected bool) {
	route := &Route{
		Path:      path,
		Method:    method,
		Handler:   handler,
		Protected: protected,
	}
	httpServer.AllRoutes = append(httpServer.AllRoutes, route)
}

// RegisterRoutes 注册路由到 Gin Engine
func registerRoutes(group *gin.RouterGroup, route *Route, httpServer *HttpServer) {
	method := strings.ToUpper(route.Method)
	if group == nil {
		switch method {
		case "GET":
			httpServer.Energy.GET(route.Path, route.Handler)
		case "POST":
			httpServer.Energy.POST(route.Path, route.Handler)
		case "PUT":
			httpServer.Energy.PUT(route.Path, route.Handler)
		case "DELETE":
			httpServer.Energy.DELETE(route.Path, route.Handler)
		default:
			logger.Error(httpServer.Name, " Invalid HTTP method", "method", route.Method)
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
			logger.Error(httpServer.Name, " Invalid HTTP method", "method", route.Method)
		}
	}
	logger.Info(httpServer.Name, " Route registered, method:", route.Method, ", path:", route.Path)

}
