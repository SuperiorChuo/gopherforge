package system

// TableInfo is one candidate table shown by the generator.
type TableInfo struct {
	Name          string `json:"name"`
	Comment       string `json:"comment"`
	PrimaryKey    string `json:"primary_key"`
	ColumnCount   int    `json:"column_count"`
	RelationCount int    `json:"relation_count"`
}

// ColumnInfo describes one column with mapped Go / TS types.
type ColumnInfo struct {
	Name         string `json:"name"`
	DBType       string `json:"db_type"`
	GoType       string `json:"go_type"`
	TSType       string `json:"ts_type"`
	Nullable     bool   `json:"nullable"`
	PrimaryKey   bool   `json:"primary_key"`
	GoField      string `json:"go_field"`
	Label        string `json:"label"`
	Comment      string `json:"comment"`
	DefaultValue string `json:"default_value,omitempty"`
}

type FormComponent string

const (
	ComponentInput    FormComponent = "input"
	ComponentTextArea FormComponent = "textarea"
	ComponentNumber   FormComponent = "number"
	ComponentSwitch   FormComponent = "switch"
	ComponentDate     FormComponent = "date"
	ComponentDateTime FormComponent = "datetime"
	ComponentSelect   FormComponent = "select"
)

// FieldConfig is the per-field generation choice from the UI.
type FieldConfig struct {
	Name      string        `json:"name"`
	Label     string        `json:"label"`
	InList    bool          `json:"in_list"`
	InSearch  bool          `json:"in_search"`
	InForm    bool          `json:"in_form"`
	Required  bool          `json:"required"`
	DictType  string        `json:"dict_type"`
	Component FormComponent `json:"component,omitempty"`
}

const (
	TplTypeCRUD = "crud"
	TplTypeTree = "tree"
	TplTypeSub  = "sub"
)

type TreeConfig struct {
	ParentField string `json:"parent_field"`
	NameField   string `json:"name_field"`
	SortField   string `json:"sort_field"`
}

type SubFieldConfig struct {
	FieldConfig
}

type SubConfig struct {
	Table   string           `json:"table"`
	FKField string           `json:"fk_field"`
	Fields  []SubFieldConfig `json:"fields,omitempty"`
}

type M2MConfig struct {
	Name         string `json:"name"`
	JoinTable    string `json:"join_table"`
	FKField      string `json:"fk_field"`
	TargetTable  string `json:"target_table"`
	TargetFK     string `json:"target_fk"`
	DisplayField string `json:"display_field"`
	Label        string `json:"label"`
}

type GenerateRequest struct {
	Table   string        `json:"table"`
	Module  string        `json:"module"`
	Title   string        `json:"title"`
	TplType string        `json:"tpl_type"`
	Tree    *TreeConfig   `json:"tree,omitempty"`
	Sub     *SubConfig    `json:"sub,omitempty"`
	Fields  []FieldConfig `json:"fields"`
	M2Ms    []M2MConfig   `json:"m2ms,omitempty"`
}

type GeneratedFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type ForeignKeyInfo struct {
	Name        string `json:"name"`
	Field       string `json:"field"`
	TargetTable string `json:"target_table"`
	TargetField string `json:"target_field"`
}

type RelationKind string

const (
	RelationManyToOne  RelationKind = "many_to_one"
	RelationOneToMany  RelationKind = "one_to_many"
	RelationManyToMany RelationKind = "many_to_many"
)

type RelationCandidate struct {
	Kind        RelationKind `json:"kind"`
	SourceTable string       `json:"source_table"`
	TargetTable string       `json:"target_table"`
	JoinTable   string       `json:"join_table,omitempty"`
	FKField     string       `json:"fk_field"`
	TargetFK    string       `json:"target_fk,omitempty"`
}

type TableSchema struct {
	Name        string              `json:"name"`
	Comment     string              `json:"comment"`
	PrimaryKey  string              `json:"primary_key"`
	Columns     []ColumnInfo        `json:"columns"`
	ForeignKeys []ForeignKeyInfo    `json:"foreign_keys"`
	Relations   []RelationCandidate `json:"relations"`
}

func (s TableSchema) HasColumn(name string) bool {
	for _, column := range s.Columns {
		if column.Name == name {
			return true
		}
	}
	return false
}

func (s TableSchema) HasManyToMany(config M2MConfig) bool {
	for _, relation := range s.Relations {
		if relation.Kind == RelationManyToMany &&
			relation.JoinTable == config.JoinTable &&
			relation.FKField == config.FKField &&
			relation.TargetTable == config.TargetTable &&
			relation.TargetFK == config.TargetFK {
			return true
		}
	}
	return false
}

type ValidatedRequest struct {
	Request   GenerateRequest
	Schemas   []TableSchema
	Schema    TableSchema
	SubSchema *TableSchema
}
