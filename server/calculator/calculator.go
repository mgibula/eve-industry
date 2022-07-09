package calculator

import (
	"math"
	"sort"

	"github.com/mgibula/eve-industry/server/db"
	"gorm.io/gorm"
)

type MaterialCalculator struct {
	EveDB             *gorm.DB
	BlueprintSettings map[uint64]BlueprintSettings
	Materials         map[uint64]*Material
}

type Material struct {
	parent               *MaterialCalculator
	BlueprintInfo        *db.EVEBlueprint
	MaterialID           uint64
	MaterialName         string
	Quantity             int64
	PrimaryProduction    int64
	Submaterials         []db.EVEMaterial
	SubmaterialQuantites map[uint64]int64
	Jobs                 []int64
	Excess               int64
}

type MaterialInfo struct {
	MaterialID          uint64
	MaterialName        string
	Quantity            int64
	MaterialBlueprintID uint64
	IsBuilt             bool
}

type JobInfo struct {
	BlueprintID     uint64
	BlueprintName   string
	Runs            int64
	Jobs            []int64
	ProductID       uint64
	ProductQuantity int64
	ME              int32
	PE              int32
}

type BlueprintSettings struct {
	Blueprint db.EVEBlueprint
	ME        int32
	PE        int32
	Decryptor uint64
	IsPrimary bool
}

func NewMaterialCalculator() MaterialCalculator {
	result := MaterialCalculator{
		BlueprintSettings: make(map[uint64]BlueprintSettings),
		Materials:         make(map[uint64]*Material),
		EveDB:             db.OpenEveDatabase(),
	}

	return result
}

func (c *MaterialCalculator) AddBlueprintSettings(blueprintID uint64, me int32, pe int32, decryptor uint64, isPrimary bool) {
	blueprint := BlueprintSettings{
		ME:        me,
		PE:        pe,
		Decryptor: decryptor,
		IsPrimary: isPrimary,
	}

	c.EveDB.Where("ID = ?", blueprintID).Take(&blueprint.Blueprint)
	c.BlueprintSettings[blueprintID] = blueprint
}

func (c *MaterialCalculator) GetBlueprintSettings(blueprintID uint64) *BlueprintSettings {
	if value, exists := c.BlueprintSettings[blueprintID]; exists {
		return &value
	}

	return nil
}

func (c *MaterialCalculator) AddQuantity(itemID uint64, name string, quantity int64, is_primary bool) {
	if _, exists := c.Materials[itemID]; !exists {
		material := Material{
			MaterialID:           itemID,
			MaterialName:         name,
			SubmaterialQuantites: make(map[uint64]int64),
			Jobs:                 make([]int64, 0),
			parent:               c,
		}

		var blueprint db.EVEBlueprint
		err := c.EveDB.Where("manufacturing_product_id = ?", itemID).Take(&blueprint).Error
		if err == nil {
			material.BlueprintInfo = &blueprint
			c.EveDB.Where("blueprint_id = ? and activity_id in (1, 11)", blueprint.ID).Find(&material.Submaterials)
		}

		c.Materials[itemID] = &material
	}

	material := c.Materials[itemID]
	material.addQuantity(quantity, is_primary)
}

func (c *MaterialCalculator) GetMaterialInfo(itemID uint64) *Material {
	return c.Materials[itemID]
}

func (c *MaterialCalculator) GetExcess() []MaterialInfo {
	result := make([]MaterialInfo, 0)

	for _, material := range c.Materials {
		excess := material.Excess
		if excess == 0 {
			continue
		}

		info := MaterialInfo{
			MaterialID:   material.MaterialID,
			MaterialName: material.MaterialName,
			Quantity:     excess,
			IsBuilt:      true,
		}

		if material.BlueprintInfo != nil {
			info.MaterialBlueprintID = material.BlueprintInfo.ID
		}

		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Quantity > result[j].Quantity
	})

	return result
}

func (c *MaterialCalculator) GetMaterials() []MaterialInfo {
	result := make([]MaterialInfo, 0)

	for _, requiredMaterial := range c.Materials {
		info := MaterialInfo{
			MaterialID:          requiredMaterial.MaterialID,
			MaterialName:        requiredMaterial.MaterialName,
			Quantity:            requiredMaterial.Quantity,
			MaterialBlueprintID: 0,
		}

		if requiredMaterial.BlueprintInfo != nil {
			info.MaterialBlueprintID = requiredMaterial.BlueprintInfo.ID
			info.IsBuilt = (c.GetBlueprintSettings(requiredMaterial.BlueprintInfo.ID) != nil)
		}

		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Quantity > result[j].Quantity
	})

	return result
}

func (c *MaterialCalculator) GetMaterialsFor(itemID uint64) []MaterialInfo {
	result := make([]MaterialInfo, 0)

	calculatedMaterials := c.Materials[itemID].getCalculatedMaterialsforProduction()

	for _, requiredMaterial := range c.Materials[itemID].Submaterials {
		info := MaterialInfo{
			MaterialID:          requiredMaterial.MaterialId,
			MaterialName:        requiredMaterial.MaterialName,
			Quantity:            calculatedMaterials[requiredMaterial.MaterialId],
			MaterialBlueprintID: requiredMaterial.MaterialBlueprintId,
			IsBuilt:             (c.GetBlueprintSettings(requiredMaterial.MaterialBlueprintId) != nil),
		}

		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Quantity > result[j].Quantity
	})

	return result
}

func (c *MaterialCalculator) GetJobsInfo() []JobInfo {
	result := make([]JobInfo, 0)

	for _, material := range c.Materials {
		if material.BlueprintInfo == nil {
			continue
		}

		settings := c.GetBlueprintSettings(material.BlueprintInfo.ID)
		if settings == nil {
			continue
		}

		info := JobInfo{
			BlueprintID:     material.BlueprintInfo.ID,
			BlueprintName:   material.BlueprintInfo.Name,
			Runs:            material.GetTotalRuns(),
			Jobs:            material.GetRequiredJobs(),
			ProductID:       material.MaterialID,
			ProductQuantity: material.GetTotalRuns() * material.BlueprintInfo.ManufacturingProductOutputQuantity,
			ME:              settings.ME,
			PE:              settings.PE,
		}

		result = append(result, info)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ProductQuantity > result[j].ProductQuantity
	})

	return result
}

func (material *Material) neededJobs() []int64 {
	result := make([]int64, 0)
	quantityNeeded := material.neededQuantity()

	for quantityNeeded > 0 {
		runsRequired := max(1, int64(math.Ceil(float64(quantityNeeded)/float64(material.BlueprintInfo.ManufacturingProductOutputQuantity))))
		runsQueued := runsRequired

		// We assume T1 BPO and max runs BPC for others
		if !material.BlueprintInfo.IsTech1() {
			runsQueued = min(runsQueued, material.BlueprintInfo.ManufacturingMaxRuns)
		}

		quantityNeeded -= runsQueued * material.BlueprintInfo.ManufacturingProductOutputQuantity
		result = append(result, runsQueued)
	}

	return result
}

func (material *Material) neededQuantity() int64 {
	return max(material.Quantity, material.PrimaryProduction)
}

func (material *Material) GetRequiredJobs() []int64 {
	return material.Jobs
}

func (material *Material) GetTotalRuns() int64 {
	var result int64

	for _, runs := range material.Jobs {
		result += runs
	}

	return result
}

func (material *Material) GetBuildibleSubmaterials() int64 {
	var result int64

	for _, submaterial := range material.Submaterials {
		if submaterial.MaterialBlueprintId > 0 {
			result++
		}
	}

	return result
}

func (material *Material) GetBuiltSubmaterials() int64 {
	var result int64

	for _, submaterial := range material.Submaterials {
		if submaterial.MaterialBlueprintId > 0 && material.parent.GetBlueprintSettings(submaterial.MaterialBlueprintId) != nil {
			result++
		}
	}

	return result
}

func (material *Material) getSubmaterialsInfoForProduction() []db.EVEMaterial {
	return material.Submaterials
}

func (material *Material) getCalculatedMaterialsforProduction() map[uint64]int64 {
	if material.parent.GetBlueprintSettings(material.BlueprintInfo.ID) == nil {
		result := make(map[uint64]int64)
		result[material.MaterialID] = material.Quantity
		return result
	} else {
		return material.SubmaterialQuantites
	}
}

func (material *Material) addQuantity(quantity int64, is_primary bool) {
	material.Quantity += quantity

	if is_primary {
		material.PrimaryProduction += quantity
	}

	if material.BlueprintInfo == nil {
		return
	}

	settings := material.parent.GetBlueprintSettings(material.BlueprintInfo.ID)
	if settings == nil {
		return
	}

	material.Jobs = material.neededJobs()
	material.Excess = material.GetTotalRuns()*material.BlueprintInfo.ManufacturingProductOutputQuantity - material.neededQuantity()

	for _, submaterial := range material.Submaterials {
		old_quantity := material.SubmaterialQuantites[submaterial.MaterialId]
		new_quantity := int64(0)

		for _, runs := range material.Jobs {
			new_quantity += max(int64(math.Ceil(float64(submaterial.Quantity)*float64(runs)*(1.0-float64(settings.ME)*0.01))), runs)
		}

		material.SubmaterialQuantites[submaterial.MaterialId] = new_quantity
		material.parent.AddQuantity(submaterial.MaterialId, submaterial.MaterialName, new_quantity-old_quantity, false)
	}
}

func max(x, y int64) int64 {
	if x > y {
		return x
	}
	return y
}

func min(x, y int64) int64 {
	if x > y {
		return y
	}
	return x
}
