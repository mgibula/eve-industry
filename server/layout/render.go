package layout

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/mgibula/eve-industry/server/db"
	"github.com/mgibula/eve-industry/server/sessions"
)

func Render(c *gin.Context, path string, values map[string]any) {
	session := sessions.OpenSession(c)

	available, exists := session.Get("available_users").([]db.ESIUser)

	if exists {
		current, _ := session.Get("current_user").(db.ESIUser)
		values["current"] = current
		values["available"] = available
	}

	c.HTML(http.StatusOK, path, values)
}
