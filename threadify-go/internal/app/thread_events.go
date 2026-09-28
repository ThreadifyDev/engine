package app

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go"
	natsrepo "github.com/threadify/engine/internal/repository/nats"
	"github.com/threadify/engine/internal/repository/valkey"
	sharedauth "threadify-go/shared/auth"
)

// threadEvents streams invalidation signals only. The viewer fetches thread
// data through GraphQL, which applies its normal field-level permissions.
func threadEvents(threadRepo *valkey.ThreadRepository, broker *nats.Conn) gin.HandlerFunc {
	return func(c *gin.Context) {
		threadID := c.Param("id")
		companyID := c.GetString(sharedauth.CtxCompanyID)
		if companyID == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		thread, err := threadRepo.GetThreadWithPermissionCheck(c.Request.Context(), threadID, companyID)
		if err != nil || thread == nil {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		updates := make(chan []byte, 256)
		var overflow atomic.Bool
		sub, err := broker.Subscribe(natsrepo.ThreadUpdateSubject(threadID), func(msg *nats.Msg) {
			select {
			case updates <- msg.Data:
			default:
				overflow.Store(true)
			}
		})
		if err != nil {
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		defer sub.Unsubscribe()
		flushCtx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		err = broker.FlushWithContext(flushCtx)
		cancel()
		if err != nil {
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}
		// The server's ordinary 30-second write deadline would otherwise force
		// healthy EventSource connections to reconnect every 30 seconds.
		if err := http.NewResponseController(c.Writer).SetWriteDeadline(time.Time{}); err != nil {
			c.AbortWithStatus(http.StatusServiceUnavailable)
			return
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache, no-transform")
		c.Header("X-Accel-Buffering", "no")
		c.Header("Connection", "keep-alive")
		fmt.Fprint(c.Writer, "retry: 5000\nevent: ready\ndata: {}\n\n")
		c.Writer.Flush()

		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()
		// Reconnect periodically so revocation and browser-session expiry are checked.
		maxSession := time.NewTimer(5 * time.Minute)
		defer maxSession.Stop()
		for {
			select {
			case <-c.Request.Context().Done():
				return
			case <-maxSession.C:
				return
			case <-heartbeat.C:
				if !broker.IsConnected() {
					return
				}
				fmt.Fprint(c.Writer, ": keepalive\n\n")
			case data := <-updates:
				if overflow.Swap(false) {
					fmt.Fprint(c.Writer, "event: update\ndata: {\"type\":\"resync\"}\n\n")
				} else {
					fmt.Fprintf(c.Writer, "event: update\ndata: %s\n\n", data)
				}
			}
			c.Writer.Flush()
		}
	}
}
