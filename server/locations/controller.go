package locations

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/mgibula/eve-industry/server/db"
	"github.com/mgibula/eve-industry/server/layout"
	"gorm.io/gorm"
)

func RegisterRoutes(c *gin.Engine) {
	c.GET("/production/locations", indexHandler)
	c.GET("/production/locations/list", listLocationsHandler)
	c.GET("/production/list-systems", listSystemsHandler)
	c.GET("/production/list-stations", listStationsHandler)
	c.POST("/production/locations/add", addLocationHandler)
	c.GET("/production/locations/remove/:id", removeLocationHandler)
}

func indexHandler(c *gin.Context) {
	layout.Render(c, "default/locations.tmpl", gin.H{})
}

func listSystemsHandler(c *gin.Context) {
	phrase := c.Query("q")

	var systems []db.EVESystem

	db := db.OpenEveDatabase()
	db.Where("system_name like ?", phrase+"%").Find(&systems)

	result := make([]string, len(systems))
	for i, system := range systems {
		result[i] = system.SystemName
	}

	c.JSON(http.StatusOK, result)
}

func listStationsHandler(c *gin.Context) {
	phrase := c.Query("system_name")

	var stations []db.EVEStation
	var system db.EVESystem

	db := db.OpenEveDatabase()
	query := db.Where("system_name = ?", phrase).First(&system)
	if errors.Is(query.Error, gorm.ErrRecordNotFound) {
		c.AbortWithStatus(http.StatusNotFound)
		return
	}

	db.Where("system_id = ?", system.ID).Order("station_name").Find(&stations)
	result := make(map[uint64]string)
	for _, station := range stations {
		result[station.ID] = station.StationName
	}

	c.JSON(http.StatusOK, result)
}

func listLocationsHandler(c *gin.Context) {
	type location struct {
		db.Location
		SystemName     *string
		StationName    *string
		SecurityStatus float32
	}

	var locations []location

	evedb := db.OpenEveDatabase()
	evedb.Model(&db.Location{}).
		Select("locations.*, eve_systems.system_name, eve_stations.station_name, eve_systems.security_status").
		Joins("left outer join eve_systems on locations.system_id = eve_systems.id").
		Joins("left outer join eve_stations on locations.station_id = eve_stations.id").
		Order("id").
		Scan(&locations)

	c.JSON(http.StatusOK, locations)
}

func addLocationHandler(c *gin.Context) {
	type params struct {
		SystemName string `form:"system_name"`
		StationId  uint64 `form:"station_id"`
		Label      string `form:"label"`
		Hangar     uint32 `format:"hangar"`
	}

	form := params{}
	c.Bind(&form)

	maybe_user, logged := c.Get("user")
	if !logged {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	user := maybe_user.(db.ESIUser)

	var location db.Location
	location.CharacterId = user.ID

	evedb := db.OpenEveDatabase()
	if len(form.SystemName) > 0 {
		var system db.EVESystem
		err := evedb.Where("system_name = ?", form.SystemName).Take(&system).Error
		if err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		location.SystemId = system.ID
	}

	if form.StationId > 0 {
		var station db.EVEStation
		err := evedb.Take(&station, form.StationId).Error
		if err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		location.StationId = station.ID
	}

	evedb.Create(&location)
	c.Redirect(http.StatusFound, "/production/locations")
}

func removeLocationHandler(c *gin.Context) {
	id := c.Param("id")

	var location db.Location
	maybe_id, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	maybe_user, logged := c.Get("user")
	if !logged {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	user := maybe_user.(db.ESIUser)

	location.ID = uint(maybe_id)

	evedb := db.OpenEveDatabase()
	err = evedb.Take(&location).Error
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	if location.CharacterId != user.ID {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	evedb.Delete(&location)

	c.Redirect(http.StatusFound, "/production/locations")
}
