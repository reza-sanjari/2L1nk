package logger

import (
	"fmt"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func BasicLoggerConfig() middleware.RequestLoggerConfig {
	return middleware.RequestLoggerConfig{
		LogURI:     true,
		LogStatus:  true,
		LogMethod:  true,
		LogLatency: true,
		LogHost:    true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			fmt.Fprintf(os.Stdout,
				"%s INFO REQUEST method=%s uri=%s status=%d latency=%s host=%s\n",
				time.Now().Format("2006/01/02 15:04:05"),
				v.Method,
				v.URI,
				v.Status,
				v.Latency.String(),
				v.Host,
			)
			return nil
		},
	}
}

func MinimalLoggerConfig() middleware.RequestLoggerConfig {
	return middleware.RequestLoggerConfig{
		LogURI:     true,
		LogStatus:  true,
		LogMethod:  true,
		LogLatency: true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			fmt.Fprintf(os.Stdout,
				"%s INFO REQUEST method=%s uri=%s status=%d latency=%s\n",
				time.Now().Format("2006/01/02 15:04:05"),
				v.Method,
				v.URI,
				v.Status,
				v.Latency.String(),
			)
			return nil
		},
	}
}

func VerboseLoggerConfig() middleware.RequestLoggerConfig {
	return middleware.RequestLoggerConfig{
		LogURI:           true,
		LogStatus:        true,
		LogMethod:        true,
		LogLatency:       true,
		LogHost:          true,
		LogRemoteIP:      true,
		LogUserAgent:     true,
		LogContentLength: true,
		LogResponseSize:  true,
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			fmt.Fprintf(os.Stdout,
				"%s INFO REQUEST method=%s uri=%s status=%d latency=%s host=%s ip=%s user_agent=%s bytes_in=%s bytes_out=%d\n",
				v.StartTime.Format("2006/01/02 15:04:05"),
				v.Method,
				v.URI,
				v.Status,
				v.Latency.String(),
				v.Host,
				v.RemoteIP,
				v.UserAgent,
				v.ContentLength, // %s
				v.ResponseSize,  // %d
			)
			return nil
		},
	}
}
