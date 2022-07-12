package db

import (
	"encoding/gob"
	"log"
	"math"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type EVERegion struct {
	ID   uint64
	Name string
}

type EVESystem struct {
	ID             uint64
	RegionId       uint64
	SystemName     string
	SecurityStatus float32
}

type EVEStation struct {
	ID          uint64
	SystemId    uint64
	StationName string
	NPC         bool
}

type EVEBlueprint struct {
	ID                                 uint64
	Name                               string
	Manufacturing                      uint32
	TimeResearch                       uint32
	MaterialResearch                   uint32
	Copying                            uint32
	Invention                          uint32
	Reaction                           uint32
	ManufacturingProductId             uint64
	ManufacturingProductName           string
	ManufacturingProductOutputQuantity int64
	MetaGroup                          uint32
	ManufacturingMaxRuns               int64
}

const (
	MetaGroupTech1            = 1
	MetaGroupTech2            = 2
	MetaGroupStoryline        = 3
	MetaGroupFaction          = 4
	MetaGroupOfficer          = 5
	MetaGroupDeadspace        = 6
	MetaGroupTech3            = 14
	MetaGroupAbyssal          = 15
	MetaGroupPremium          = 17
	MetaGroupLimitedTime      = 19
	MetaGroupStructureFaction = 52
	MetaGroupStructureTech2   = 53
	MetaGroupStructureTech1   = 54
)

func (b *EVEBlueprint) GetDefaultME() int32 {
	if b.Reaction > 0 {
		return 0
	} else if b.MetaGroup == MetaGroupTech1 {
		return 10
	} else if b.MetaGroup == MetaGroupTech2 {
		return 2
	} else {
		return 0
	}
}

func (b *EVEBlueprint) GetDefaultPE() int32 {
	if b.Reaction > 0 {
		return 0
	} else if b.MetaGroup == MetaGroupTech1 {
		return 10
	} else if b.MetaGroup == MetaGroupTech2 {
		return 4
	} else {
		return 0
	}
}

func (b *EVEBlueprint) GetDefaultMaxRuns() int64 {
	if b.Reaction > 0 {
		return math.MaxInt64
	} else if b.MetaGroup == MetaGroupTech1 {
		return math.MaxUint32
	} else if b.MetaGroup == MetaGroupTech2 {
		return b.ManufacturingMaxRuns
	} else {
		return math.MaxInt64
	}
}

func (b *EVEBlueprint) IsTech2() bool {
	return (b.MetaGroup == MetaGroupTech2)
}

func (b *EVEBlueprint) IsTech1() bool {
	return (b.MetaGroup == MetaGroupTech1)
}

func (b *EVEBlueprint) IsReaction() bool {
	return (b.Reaction > 0)
}

type EVEMaterial struct {
	ID                              uint `gorm:"primaryKey"`
	BlueprintId                     uint64
	ActivityId                      uint32
	MaterialName                    string
	MaterialId                      uint64
	Quantity                        int64
	MaterialBlueprintId             uint64
	MaterialBlueprintOutputQuantity int64
}

type EVEDecryptor struct {
	ID                  uint64
	Name                string
	ProbabilityModifier float32
	RunsModifier        int32
	MEModifier          int32
	PEModifier          int32
}

type ESIUser struct {
	ID            uint64
	CharacterName string
	RefreshToken  string
	AccessToken   string
	ValidUntil    time.Time
}

type ESICall struct {
	ID         uint64 `gorm:"primaryKey"`
	URL        string `gorm:"index:url_idx,unique"`
	Params     string `gorm:"index:url_idx,unique"`
	Response   string
	ValidUntil time.Time
	Etag       string
}

type SystemCostIndices struct {
	ID            uint64 `gorm:"primaryKey"`
	Manufacturing float32
	MEResearch    float32
	PEResearch    float32
	Copying       float32
	Invention     float32
	Reaction      float32
}

type Location struct {
	gorm.Model
	CharacterId uint64
	SystemId    uint64
	StationId   uint64
	Hangar      uint32
	Label       string
}

func OpenEveDatabase() *gorm.DB {
	db, err := gorm.Open(sqlite.Open("resources/eve.db"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		log.Fatalln(err)
	}

	return db
}

func InitEveDatabase() {
	db := OpenEveDatabase()
	db.AutoMigrate(&EVERegion{})
	db.AutoMigrate(&EVESystem{})
	db.AutoMigrate(&EVEStation{})
	db.AutoMigrate(&EVEBlueprint{})
	db.AutoMigrate(&EVEMaterial{})
	db.AutoMigrate(&EVEDecryptor{})

	db.AutoMigrate(&ESIUser{})
	db.AutoMigrate(&ESICall{})
	db.AutoMigrate(&Location{})
	db.AutoMigrate(&SystemCostIndices{})

	gob.Register(ESICall{})
	gob.Register([]ESICall{})

	gob.Register(ESIUser{})
	gob.Register([]ESIUser{})

	gob.Register(Location{})
	gob.Register([]Location{})

	gob.Register(EVERegion{})
	gob.Register([]EVERegion{})

	gob.Register(EVESystem{})
	gob.Register([]EVESystem{})

	gob.Register(EVEStation{})
	gob.Register([]EVEStation{})

	gob.Register(EVEBlueprint{})
	gob.Register([]EVEBlueprint{})

	gob.Register(EVEMaterial{})
	gob.Register([]EVEMaterial{})

	gob.Register(EVEDecryptor{})
	gob.Register([]EVEDecryptor{})
}

func CookDatabase(path string) {
	InitEveDatabase()

	db := OpenEveDatabase()
	db.Begin()

	source, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		log.Fatalln(err)
	}

	{
		rows, err := source.Raw("SELECT regionID, regionName from mapRegions").Rows()
		if err != nil {
			log.Fatalln(err)
		}

		db.Exec("DELETE FROM eve_regions")
		var regions []EVERegion

		for rows.Next() {
			var region EVERegion
			rows.Scan(&region.ID, &region.Name)

			regions = append(regions, region)
		}
		rows.Close()

		result := db.CreateInBatches(&regions, 1000)
		if result.Error != nil {
			log.Println(result.Error)
		}

		log.Printf("EVERegions: Added %d records\n", len(regions))
	}

	{
		rows, err := source.Raw("SELECT solarSystemID, regionID, solarSystemName, security from mapSolarSystems").Rows()
		if err != nil {
			log.Fatalln(err)
		}

		db.Exec("DELETE FROM eve_systems")
		var systems []EVESystem

		for rows.Next() {
			var system EVESystem
			rows.Scan(&system.ID, &system.RegionId, &system.SystemName, &system.SecurityStatus)

			systems = append(systems, system)
		}
		rows.Close()

		result := db.CreateInBatches(&systems, 1000)
		if result.Error != nil {
			log.Println(result.Error)
		}

		log.Printf("EVESystems: Added %d records\n", len(systems))
	}

	{
		rows, err := source.Raw("SELECT stationID, solarSystemID, stationName from staStations").Rows()
		if err != nil {
			log.Fatalln(err)
		}

		db.Exec("DELETE FROM eve_stations")
		var stations []EVEStation

		for rows.Next() {
			var station EVEStation
			rows.Scan(&station.ID, &station.SystemId, &station.StationName)
			station.NPC = true

			stations = append(stations, station)
		}
		rows.Close()

		result := db.CreateInBatches(&stations, 1000)
		if result.Error != nil {
			log.Println(result.Error)
		}

		log.Printf("EVEStations: Added %d records\n", len(stations))
	}

	{
		rows, err := source.Raw(`
			select distinct ia.typeID as id,
				it.typeName as name,
				coalesce((select time from industryActivity where typeID = ia.typeID and activityID = 1), 0) as manufacturing,
				coalesce((select time from industryActivity where typeID = ia.typeID and activityID = 3), 0) as time_research,
				coalesce((select time from industryActivity where typeID = ia.typeID and activityID = 4), 0) as material_research,
				coalesce((select time from industryActivity where typeID = ia.typeID and activityID = 5), 0) as copying,
				coalesce((select time from industryActivity where typeID = ia.typeID and activityID = 8), 0) as invention,
				coalesce((select time from industryActivity where typeID = ia.typeID and activityID = 11), 0) as reactions,
				(select productTypeID from industryActivityProducts where typeID = ia.typeID and (activityID = 1 or activityID = 11)) as manufacturing_product_id,
				(select typeName from invTypes where published = '1' and typeID in (select productTypeID from industryActivityProducts where typeID = ia.typeID and (activityID = 1 or activityID = 11)) ) as manufacturing_product_name,
				(select quantity from industryActivityProducts where typeID = ia.typeID and (activityID = 1 or activityID = 11)) as manufacturing_product_output_quantity,
				COALESCE((select metaGroupID from invMetaTypes where typeID = (select productTypeID from industryActivityProducts where typeID = ia.typeID and (activityID = 1 or activityID = 11))), 1) as meta_group,
				(select maxProductionLimit from industryBlueprints where typeID = ia.typeID) as manufacturing_max_runs
			from industryActivity ia left join invTypes it using (typeID) where it.published = '1'
		`).Rows()
		if err != nil {
			log.Fatalln(err)
		}

		db.Exec("DELETE FROM eve_blueprints")
		var blueprints []EVEBlueprint

		for rows.Next() {
			var blueprint EVEBlueprint
			rows.Scan(&blueprint.ID,
				&blueprint.Name,
				&blueprint.Manufacturing,
				&blueprint.TimeResearch,
				&blueprint.MaterialResearch,
				&blueprint.Copying,
				&blueprint.Invention,
				&blueprint.Reaction,
				&blueprint.ManufacturingProductId,
				&blueprint.ManufacturingProductName,
				&blueprint.ManufacturingProductOutputQuantity,
				&blueprint.MetaGroup,
				&blueprint.ManufacturingMaxRuns,
			)

			blueprints = append(blueprints, blueprint)
		}
		rows.Close()

		result := db.CreateInBatches(&blueprints, 1000)
		if result.Error != nil {
			log.Println(result.Error)
		}

		log.Printf("EVEBlueprints: Added %d records\n", len(blueprints))
	}

	{
		rows, err := source.Raw(`
			select iam.typeID as blueprint_id,
				activityID as activity_id,
				typeName as material_name,
				materialTypeID as material_id,
				quantity,
				(select iam2.typeID from industryActivityProducts iam2 left join invTypes it2 on (iam2.typeID = it2.typeID) where it2.published = '1' and (activityID = 1 or activityID = 11) and productTypeID = materialTypeID) as material_blueprint_id,
				(select quantity from industryActivityProducts where (activityID = 1 or activityID = 11) and productTypeID = materialTypeID) as material_blueprint_output_quantity
			from industryActivityMaterials iam left join invTypes it on (iam.materialTypeID = it.typeID) where it.published = '1'
		`).Rows()
		if err != nil {
			log.Fatalln(err)
		}

		db.Exec("DELETE FROM eve_materials")
		var materials []EVEMaterial

		for rows.Next() {
			var material EVEMaterial
			rows.Scan(&material.BlueprintId,
				&material.ActivityId,
				&material.MaterialName,
				&material.MaterialId,
				&material.Quantity,
				&material.MaterialBlueprintId,
				&material.MaterialBlueprintOutputQuantity,
			)

			materials = append(materials, material)
		}
		rows.Close()

		result := db.CreateInBatches(&materials, 1000)
		if result.Error != nil {
			log.Println(result.Error)
		}

		log.Printf("EVEMaterial: Added %d records\n", len(materials))
	}

	{
		db.Exec("DELETE FROM eve_decryptors")
		decryptors := []EVEDecryptor{
			{
				ID:                  34201,
				Name:                "Accelerant Decryptor",
				ProbabilityModifier: 1.2,
				RunsModifier:        1,
				MEModifier:          2,
				PEModifier:          10,
			},
			{
				ID:                  34202,
				Name:                "Attainment Decryptor",
				ProbabilityModifier: 1.8,
				RunsModifier:        4,
				MEModifier:          -1,
				PEModifier:          4,
			},
			{
				ID:                  34203,
				Name:                "Augmentation Decryptor",
				ProbabilityModifier: 0.6,
				RunsModifier:        9,
				MEModifier:          -2,
				PEModifier:          2,
			},
			{
				ID:                  34204,
				Name:                "Parity Decryptor",
				ProbabilityModifier: 1.5,
				RunsModifier:        3,
				MEModifier:          1,
				PEModifier:          -2,
			},
			{
				ID:                  34205,
				Name:                "Process Decryptor",
				ProbabilityModifier: 1.1,
				RunsModifier:        0,
				MEModifier:          1,
				PEModifier:          8,
			},
			{
				ID:                  34206,
				Name:                "Symmetry Decryptor",
				ProbabilityModifier: 1.0,
				RunsModifier:        2,
				MEModifier:          1,
				PEModifier:          8,
			},
			{
				ID:                  34207,
				Name:                "Optimized Attainment Decryptor",
				ProbabilityModifier: 1.9,
				RunsModifier:        2,
				MEModifier:          1,
				PEModifier:          -2,
			},
			{
				ID:                  34208,
				Name:                "Optimized Augmentation Decryptor",
				ProbabilityModifier: 0.9,
				RunsModifier:        7,
				MEModifier:          2,
				PEModifier:          0,
			},
		}

		result := db.Create(&decryptors)
		if result.Error != nil {
			log.Println(result.Error)
		}

		log.Printf("EVEDecryptors: Added %d records\n", len(decryptors))
	}

	db.Commit()
}
