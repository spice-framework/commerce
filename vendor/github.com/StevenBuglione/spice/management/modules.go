package management

import (
	"fmt"
	"slices"
	"strings"
)

const moduleReportSchema = "spice.modules/v1"

// ModuleDefinition is generated input for one validated application module.
type ModuleDefinition struct {
	ID                  string
	RootPackage         string
	Packages            []string
	NamedInterfaces     []NamedInterface
	AllowedDependencies []string
}

// NamedInterface identifies one explicitly exported descendant package.
type NamedInterface struct {
	Name        string `json:"name"`
	PackagePath string `json:"package"`
}

// ApplicationModule is one deterministic client-facing module canvas.
type ApplicationModule struct {
	ID                   string           `json:"id"`
	RootPackage          string           `json:"root_package"`
	Packages             []string         `json:"packages"`
	DefaultAPI           string           `json:"default_api"`
	NamedInterfaces      []NamedInterface `json:"named_interfaces"`
	AllowedDependencies  []string         `json:"allowed_dependencies"`
	ObservedDependencies []string         `json:"observed_dependencies"`
}

// ModuleEdge is one observed cross-module Go import.
type ModuleEdge struct {
	FromModule  string `json:"from_module"`
	ToModule    string `json:"to_module"`
	API         string `json:"api"`
	FromPackage string `json:"from_package"`
	ToPackage   string `json:"to_package"`
}

// ModuleReport is the portable runtime application-module canvas.
type ModuleReport struct {
	Schema             string              `json:"schema"`
	Modules            []ApplicationModule `json:"modules"`
	Edges              []ModuleEdge        `json:"edges"`
	UnassignedPackages []string            `json:"unassigned_packages"`
}

// NewModuleReport validates and copies generated module metadata. The returned
// report uses the same schema and ordering as the Spice module JSON canvas.
func NewModuleReport(
	definitions []ModuleDefinition,
	edges []ModuleEdge,
	unassignedPackages []string,
) (ModuleReport, error) {
	modules, packages, err := normalizeModuleDefinitions(definitions)
	if err != nil {
		return ModuleReport{}, err
	}
	normalizedEdges, observed, err := normalizeModuleEdges(
		edges,
		modules,
		packages,
	)
	if err != nil {
		return ModuleReport{}, err
	}
	unassigned, err := normalizeUnassignedPackages(
		unassignedPackages,
		packages,
	)
	if err != nil {
		return ModuleReport{}, err
	}
	for index := range modules {
		modules[index].ObservedDependencies = observed[modules[index].ID]
		if modules[index].ObservedDependencies == nil {
			modules[index].ObservedDependencies = []string{}
		}
	}
	return ModuleReport{
		Schema:             moduleReportSchema,
		Modules:            modules,
		Edges:              normalizedEdges,
		UnassignedPackages: unassigned,
	}, nil
}

func normalizeModuleDefinitions(
	definitions []ModuleDefinition,
) ([]ApplicationModule, map[string]string, error) {
	modules := make([]ApplicationModule, 0, len(definitions))
	packages := make(map[string]string)
	ids := make(map[string]struct{}, len(definitions))
	for index, definition := range definitions {
		if definition.ID == "" {
			return nil, nil, fmt.Errorf(
				"construct module report: module %d has an empty ID",
				index,
			)
		}
		if _, duplicate := ids[definition.ID]; duplicate {
			return nil, nil, fmt.Errorf(
				"construct module report: module ID %q is duplicated",
				definition.ID,
			)
		}
		ids[definition.ID] = struct{}{}
		module, err := normalizeModuleDefinition(definition, packages)
		if err != nil {
			return nil, nil, err
		}
		modules = append(modules, module)
	}
	slices.SortFunc(modules, func(left, right ApplicationModule) int {
		return strings.Compare(left.ID, right.ID)
	})
	for _, module := range modules {
		for _, dependency := range module.AllowedDependencies {
			target, _, _ := strings.Cut(dependency, "::")
			if _, found := ids[target]; !found {
				return nil, nil, fmt.Errorf(
					"construct module report: module %q allows unknown dependency %q",
					module.ID,
					dependency,
				)
			}
		}
	}
	return modules, packages, nil
}

func normalizeModuleDefinition(
	definition ModuleDefinition,
	packageOwners map[string]string,
) (ApplicationModule, error) {
	if definition.RootPackage == "" {
		return ApplicationModule{}, fmt.Errorf(
			"construct module report: module %q has an empty root package",
			definition.ID,
		)
	}
	packages, err := sortedUniqueStrings(
		"module "+definition.ID+" package",
		definition.Packages,
	)
	if err != nil {
		return ApplicationModule{}, err
	}
	if !slices.Contains(packages, definition.RootPackage) {
		return ApplicationModule{}, fmt.Errorf(
			"construct module report: module %q packages omit root %q",
			definition.ID,
			definition.RootPackage,
		)
	}
	for _, packagePath := range packages {
		if owner, duplicate := packageOwners[packagePath]; duplicate {
			return ApplicationModule{}, fmt.Errorf(
				"construct module report: package %q belongs to both %q and %q",
				packagePath,
				owner,
				definition.ID,
			)
		}
		packageOwners[packagePath] = definition.ID
	}
	interfaces, err := normalizeNamedInterfaces(
		definition.ID,
		definition.NamedInterfaces,
		packages,
	)
	if err != nil {
		return ApplicationModule{}, err
	}
	allowed, err := sortedUniqueStrings(
		"module "+definition.ID+" allowed dependency",
		definition.AllowedDependencies,
	)
	if err != nil {
		return ApplicationModule{}, err
	}
	for _, dependency := range allowed {
		moduleID, interfaceName, found := strings.Cut(dependency, "::")
		if moduleID == "" ||
			(found && (interfaceName == "" || strings.Contains(interfaceName, "::"))) {
			return ApplicationModule{}, fmt.Errorf(
				"construct module report: module %q has invalid allowed dependency %q",
				definition.ID,
				dependency,
			)
		}
	}
	return ApplicationModule{
		ID:                   definition.ID,
		RootPackage:          definition.RootPackage,
		Packages:             packages,
		DefaultAPI:           definition.RootPackage,
		NamedInterfaces:      interfaces,
		AllowedDependencies:  allowed,
		ObservedDependencies: []string{},
	}, nil
}

func normalizeNamedInterfaces(
	moduleID string,
	interfaces []NamedInterface,
	packages []string,
) ([]NamedInterface, error) {
	result := append([]NamedInterface(nil), interfaces...)
	slices.SortFunc(result, func(left, right NamedInterface) int {
		if compared := strings.Compare(left.Name, right.Name); compared != 0 {
			return compared
		}
		return strings.Compare(left.PackagePath, right.PackagePath)
	})
	seen := make(map[string]struct{}, len(result))
	for _, item := range result {
		if item.Name == "" || item.PackagePath == "" {
			return nil, fmt.Errorf(
				"construct module report: module %q has an incomplete named interface",
				moduleID,
			)
		}
		if _, duplicate := seen[item.Name]; duplicate {
			return nil, fmt.Errorf(
				"construct module report: module %q duplicates named interface %q",
				moduleID,
				item.Name,
			)
		}
		if !slices.Contains(packages, item.PackagePath) {
			return nil, fmt.Errorf(
				"construct module report: module %q named interface %q uses unowned package %q",
				moduleID,
				item.Name,
				item.PackagePath,
			)
		}
		seen[item.Name] = struct{}{}
	}
	if result == nil {
		return []NamedInterface{}, nil
	}
	return result, nil
}

func normalizeModuleEdges(
	edges []ModuleEdge,
	modules []ApplicationModule,
	packageOwners map[string]string,
) ([]ModuleEdge, map[string][]string, error) {
	moduleByID := make(map[string]ApplicationModule, len(modules))
	allowed := make(map[string]map[string]struct{}, len(modules))
	for _, module := range modules {
		moduleByID[module.ID] = module
		allowed[module.ID] = make(map[string]struct{}, len(module.AllowedDependencies))
		for _, dependency := range module.AllowedDependencies {
			allowed[module.ID][dependency] = struct{}{}
		}
	}
	result := append([]ModuleEdge(nil), edges...)
	slices.SortFunc(result, compareModuleEdges)
	observedSets := make(map[string]map[string]struct{}, len(modules))
	for index := range result {
		edge := result[index]
		if index != 0 && compareModuleEdges(result[index-1], edge) == 0 {
			return nil, nil, fmt.Errorf(
				"construct module report: edge %d is duplicated",
				index,
			)
		}
		dependency, err := validateModuleEdge(
			index,
			edge,
			moduleByID,
			packageOwners,
			allowed,
		)
		if err != nil {
			return nil, nil, err
		}
		if observedSets[edge.FromModule] == nil {
			observedSets[edge.FromModule] = make(map[string]struct{})
		}
		observedSets[edge.FromModule][dependency] = struct{}{}
	}
	observed := make(map[string][]string, len(observedSets))
	for moduleID, dependencies := range observedSets {
		for dependency := range dependencies {
			observed[moduleID] = append(observed[moduleID], dependency)
		}
		slices.Sort(observed[moduleID])
	}
	if result == nil {
		result = []ModuleEdge{}
	}
	return result, observed, nil
}

func validateModuleEdge(
	index int,
	edge ModuleEdge,
	modules map[string]ApplicationModule,
	packageOwners map[string]string,
	allowed map[string]map[string]struct{},
) (string, error) {
	if _, found := modules[edge.FromModule]; !found {
		return "", fmt.Errorf(
			"construct module report: edge %d has unknown source module %q",
			index,
			edge.FromModule,
		)
	}
	if _, found := modules[edge.ToModule]; !found {
		return "", fmt.Errorf(
			"construct module report: edge %d has unknown target module %q",
			index,
			edge.ToModule,
		)
	}
	if edge.FromModule == edge.ToModule || edge.API == "" {
		return "", fmt.Errorf(
			"construct module report: edge %d is invalid",
			index,
		)
	}
	if packageOwners[edge.FromPackage] != edge.FromModule ||
		packageOwners[edge.ToPackage] != edge.ToModule {
		return "", fmt.Errorf(
			"construct module report: edge %d package ownership is inconsistent",
			index,
		)
	}
	dependency := edge.ToModule
	if edge.API != "default" {
		dependency += "::" + edge.API
	}
	if _, permitted := allowed[edge.FromModule][dependency]; !permitted {
		return "", fmt.Errorf(
			"construct module report: edge %d dependency %q is not allowed by module %q",
			index,
			dependency,
			edge.FromModule,
		)
	}
	return dependency, nil
}

func compareModuleEdges(left, right ModuleEdge) int {
	for _, compared := range [][2]string{
		{left.FromModule, right.FromModule},
		{left.ToModule, right.ToModule},
		{left.API, right.API},
		{left.FromPackage, right.FromPackage},
		{left.ToPackage, right.ToPackage},
	} {
		if result := strings.Compare(compared[0], compared[1]); result != 0 {
			return result
		}
	}
	return 0
}

func normalizeUnassignedPackages(
	values []string,
	packageOwners map[string]string,
) ([]string, error) {
	result, err := sortedUniqueStrings("unassigned package", values)
	if err != nil {
		return nil, err
	}
	for _, packagePath := range result {
		if owner := packageOwners[packagePath]; owner != "" {
			return nil, fmt.Errorf(
				"construct module report: package %q is both unassigned and owned by %q",
				packagePath,
				owner,
			)
		}
	}
	return result, nil
}

func sortedUniqueStrings(label string, values []string) ([]string, error) {
	result := append([]string(nil), values...)
	slices.Sort(result)
	for index, value := range result {
		if value == "" {
			return nil, fmt.Errorf(
				"construct module report: %s %d is empty",
				label,
				index,
			)
		}
		if index != 0 && value == result[index-1] {
			return nil, fmt.Errorf(
				"construct module report: %s %q is duplicated",
				label,
				value,
			)
		}
	}
	if result == nil {
		return []string{}, nil
	}
	return result, nil
}
