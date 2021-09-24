package main

import (
	"github.com/gin-gonic/gin"
	"os"
	"regexp"
	"time"
)

func checkSession(c *gin.Context, jsonResponse bool) bool {
	ginLogger(c)

	session := c.PostForm("token")
	if session == "" {
		session = c.Query("token")
	}

	if session == "" || !validSession(session) {
		oneTimeToken := c.Query("ott")
		now := time.Now()

		oneTimeTokenMutex.Lock()
		v, ok := oneTimeTokens[oneTimeToken]
		oneTimeTokenMutex.Unlock()

		if ok && now.Sub(v) < 1*time.Minute {
			oneTimeTokenMutex.Lock()
			delete(oneTimeTokens, oneTimeToken)
			oneTimeTokenMutex.Unlock()

			return true
		}

		if jsonResponse {
			c.JSON(200, map[string]interface{}{
				"status": false,
				"error":  "unauthorized",
			})
		} else {
			c.Data(401, "text/plain", []byte("Unauthorized"))
		}

		c.Abort()
		return false
	}

	return true
}

func validSession(session string) bool {
	rgx := regexp.MustCompile(`(?mi)[^a-z0-9]`)
	session = rgx.ReplaceAllString(session, "")
	if session == "" {
		return false
	}

	sessionFile := SessionDirectory + "/" + session + ".session"
	if _, err := os.Stat(sessionFile); err != nil {
		return false
	}

	return true
}
