package system

import (
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

type SchemaInspector struct {
	db *gorm.DB
}

func NewSchemaInspector(db *gorm.DB) *SchemaInspector {
	return &SchemaInspector{db: db}
}

func (s CodegenService) ValidateRequest(req GenerateRequest) (ValidatedRequest, error) {
	req.Module = strings.ToLower(strings.TrimSpace(req.Module))
	if req.Table == "" || req.Module == "" {
		return ValidatedRequest{}, fmt.Errorf("table and module are required")
	}
	if !moduleRe.MatchString(req.Module) {
		return ValidatedRequest{}, fmt.Errorf("module must be lowercase letters/digits, starting with a letter")
	}
	if req.TplType == "" {
		req.TplType = TplTypeCRUD
	}
	if req.TplType != TplTypeCRUD && req.TplType != TplTypeTree && req.TplType != TplTypeSub {
		return ValidatedRequest{}, fmt.Errorf("unknown tpl_type %q", req.TplType)
	}

	schemas, err := NewSchemaInspector(s.DB).InspectTables()
	if err != nil {
		return ValidatedRequest{}, err
	}
	schema, ok := findTableSchema(schemas, req.Table)
	if !ok {
		return ValidatedRequest{}, fmt.Errorf("table %q not found", req.Table)
	}
	if schema.PrimaryKey == "" {
		return ValidatedRequest{}, fmt.Errorf("table %s has no primary key", req.Table)
	}
	byName := make(map[string]ColumnInfo, len(schema.Columns))
	for _, column := range schema.Columns {
		byName[column.Name] = column
	}
	seen := make(map[string]bool, len(req.Fields))
	fields := make([]FieldConfig, len(req.Fields))
	copy(fields, req.Fields)
	for index := range fields {
		field := &fields[index]
		column, ok := byName[field.Name]
		if !ok {
			return ValidatedRequest{}, fmt.Errorf("field %q not found in table %s", field.Name, req.Table)
		}
		if seen[field.Name] {
			return ValidatedRequest{}, fmt.Errorf("field %q is configured more than once", field.Name)
		}
		seen[field.Name] = true
		if strings.TrimSpace(field.Label) == "" {
			field.Label = column.Label
		}
		field.DictType = strings.TrimSpace(field.DictType)
		if field.DictType != "" {
			if err := s.validateDictionaryType(field.DictType); err != nil {
				return ValidatedRequest{}, err
			}
			field.Component = ComponentSelect
		} else if field.Component == "" {
			field.Component = inferFormComponent(column, field.DictType)
		} else if !isKnownFormComponent(field.Component) {
			return ValidatedRequest{}, fmt.Errorf("unknown form component %q for field %s", field.Component, field.Name)
		}
	}
	req.Fields = fields
	var subSchema *TableSchema
	switch req.TplType {
	case TplTypeTree:
		if err := validateTreeRequest(req.Tree, schema); err != nil {
			return ValidatedRequest{}, err
		}
	case TplTypeSub:
		normalizedSub, inspectedSub, err := s.validateSubRequest(req.Sub, schema, schemas)
		if err != nil {
			return ValidatedRequest{}, err
		}
		req.Sub = normalizedSub
		subSchema = &inspectedSub
	}
	if req.TplType == TplTypeSub && len(req.M2Ms) > 0 {
		return ValidatedRequest{}, fmt.Errorf("sub mode does not support many-to-many relations")
	}
	m2ms := make([]M2MConfig, len(req.M2Ms))
	copy(m2ms, req.M2Ms)
	seenRelations := make(map[string]bool, len(m2ms))
	for index := range m2ms {
		relation := &m2ms[index]
		relation.Name = strings.TrimSpace(relation.Name)
		if !relationNameRe.MatchString(relation.Name) {
			return ValidatedRequest{}, fmt.Errorf("invalid many-to-many relation name %q", relation.Name)
		}
		if seenRelations[relation.Name] {
			return ValidatedRequest{}, fmt.Errorf("many-to-many relation %q is configured more than once", relation.Name)
		}
		seenRelations[relation.Name] = true
		if !schema.HasManyToMany(*relation) {
			return ValidatedRequest{}, fmt.Errorf("many-to-many relation through %q is not recognized for table %s", relation.JoinTable, schema.Name)
		}
		target, ok := findTableSchema(schemas, relation.TargetTable)
		if !ok {
			return ValidatedRequest{}, fmt.Errorf("relation target table %q not found", relation.TargetTable)
		}
		if !target.HasColumn(relation.DisplayField) {
			return ValidatedRequest{}, fmt.Errorf("display field %q not found in relation target %s", relation.DisplayField, relation.TargetTable)
		}
		if strings.TrimSpace(relation.Label) == "" {
			relation.Label = relation.Name
		}
	}
	req.M2Ms = m2ms
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		req.Title = exportedName(singular(req.Module))
	}
	return ValidatedRequest{Request: req, Schemas: schemas, Schema: schema, SubSchema: subSchema}, nil
}

func findTableSchema(schemas []TableSchema, name string) (TableSchema, bool) {
	index := sort.Search(len(schemas), func(index int) bool { return schemas[index].Name >= name })
	if index >= len(schemas) || schemas[index].Name != name {
		return TableSchema{}, false
	}
	return schemas[index], true
}

func validateTreeRequest(config *TreeConfig, schema TableSchema) error {
	if config == nil || config.ParentField == "" || config.NameField == "" {
		return fmt.Errorf("tree mode requires parent_field and name_field")
	}
	columns := make(map[string]ColumnInfo, len(schema.Columns))
	for _, column := range schema.Columns {
		columns[column.Name] = column
	}
	parent, ok := columns[config.ParentField]
	if !ok || parent.PrimaryKey || parent.GoType != "int64" {
		return fmt.Errorf("parent field %q must be a non-primary integer column", config.ParentField)
	}
	name, ok := columns[config.NameField]
	if !ok || name.GoType != "string" || name.PrimaryKey {
		return fmt.Errorf("name field %q must be a text column", config.NameField)
	}
	if config.SortField != "" {
		sortColumn, ok := columns[config.SortField]
		if !ok || sortColumn.PrimaryKey || sortColumn.Name == config.ParentField {
			return fmt.Errorf("sort field %q must be a regular column", config.SortField)
		}
	}
	return nil
}

func (s CodegenService) validateSubRequest(config *SubConfig, main TableSchema, schemas []TableSchema) (*SubConfig, TableSchema, error) {
	if config == nil || config.Table == "" || config.FKField == "" {
		return nil, TableSchema{}, fmt.Errorf("sub mode requires sub table and fk_field")
	}
	if config.Table == main.Name {
		return nil, TableSchema{}, fmt.Errorf("sub table must differ from main table")
	}
	sub, ok := findTableSchema(schemas, config.Table)
	if !ok {
		return nil, TableSchema{}, fmt.Errorf("sub table %q not found", config.Table)
	}
	foreignKeyFound := false
	for _, foreignKey := range sub.ForeignKeys {
		if foreignKey.Field == config.FKField && foreignKey.TargetTable == main.Name && foreignKey.TargetField == main.PrimaryKey {
			foreignKeyFound = true
			break
		}
	}
	if !foreignKeyFound {
		return nil, TableSchema{}, fmt.Errorf("sub field %q is not a foreign key to %s.%s", config.FKField, main.Name, main.PrimaryKey)
	}

	byName := make(map[string]ColumnInfo, len(sub.Columns))
	for _, column := range sub.Columns {
		byName[column.Name] = column
	}
	fields := make([]SubFieldConfig, len(config.Fields))
	copy(fields, config.Fields)
	if len(fields) == 0 {
		for _, column := range sub.Columns {
			if column.PrimaryKey || column.Name == config.FKField || isServerManagedColumn(column.Name) {
				continue
			}
			fields = append(fields, SubFieldConfig{FieldConfig: FieldConfig{
				Name: column.Name, Label: column.Label, InList: true, InForm: true,
				Component: inferFormComponent(column, ""),
			}})
		}
	}
	seen := make(map[string]bool, len(fields))
	for index := range fields {
		field := &fields[index].FieldConfig
		column, ok := byName[field.Name]
		if !ok {
			return nil, TableSchema{}, fmt.Errorf("sub field %q not found in table %s", field.Name, sub.Name)
		}
		if column.PrimaryKey || field.Name == config.FKField || isServerManagedColumn(field.Name) {
			return nil, TableSchema{}, fmt.Errorf("sub field %q cannot be configured", field.Name)
		}
		if seen[field.Name] {
			return nil, TableSchema{}, fmt.Errorf("sub field %q is configured more than once", field.Name)
		}
		seen[field.Name] = true
		if strings.TrimSpace(field.Label) == "" {
			field.Label = column.Label
		}
		field.DictType = strings.TrimSpace(field.DictType)
		if field.DictType != "" {
			if err := s.validateDictionaryType(field.DictType); err != nil {
				return nil, TableSchema{}, err
			}
			field.Component = ComponentSelect
		} else if field.Component == "" {
			field.Component = inferFormComponent(column, "")
		} else if !isKnownFormComponent(field.Component) {
			return nil, TableSchema{}, fmt.Errorf("unknown form component %q for sub field %s", field.Component, field.Name)
		}
	}
	normalized := *config
	normalized.Fields = fields
	return &normalized, sub, nil
}

func (s CodegenService) validateDictionaryType(code string) error {
	if s.DB == nil || !s.DB.Migrator().HasTable("dict_types") {
		return fmt.Errorf("dictionary type %q not found", code)
	}
	var count int64
	if err := s.DB.Table("dict_types").Where("code = ?", code).Count(&count).Error; err != nil {
		return fmt.Errorf("validate dictionary type %q: %w", code, err)
	}
	if count == 0 {
		return fmt.Errorf("dictionary type %q not found", code)
	}
	return nil
}

func inferFormComponent(column ColumnInfo, dictType string) FormComponent {
	if strings.TrimSpace(dictType) != "" {
		return ComponentSelect
	}
	switch column.GoType {
	case "int64", "float64":
		return ComponentNumber
	case "bool":
		return ComponentSwitch
	case "time.Time":
		if column.DBType == "date" {
			return ComponentDate
		}
		return ComponentDateTime
	default:
		name := strings.ToLower(column.Name)
		if strings.Contains(name, "content") || strings.Contains(name, "description") || strings.Contains(name, "remark") {
			return ComponentTextArea
		}
		return ComponentInput
	}
}

func isKnownFormComponent(component FormComponent) bool {
	switch component {
	case ComponentInput, ComponentTextArea, ComponentNumber, ComponentSwitch,
		ComponentDate, ComponentDateTime, ComponentSelect:
		return true
	default:
		return false
	}
}

func (i *SchemaInspector) InspectTable(table string) (TableSchema, error) {
	if i == nil || i.db == nil {
		return TableSchema{}, fmt.Errorf("database is not initialized")
	}
	schemas, err := i.InspectTables()
	if err != nil {
		return TableSchema{}, err
	}
	for _, schema := range schemas {
		if schema.Name == table {
			return schema, nil
		}
	}
	return TableSchema{}, fmt.Errorf("table %q not found", table)
}

func (i *SchemaInspector) InspectTables() ([]TableSchema, error) {
	if i == nil || i.db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	tables, err := i.db.Migrator().GetTables()
	if err != nil {
		return nil, err
	}
	sort.Strings(tables)
	schemas := make([]TableSchema, 0, len(tables))
	for _, table := range tables {
		if codegenExcluded[table] {
			continue
		}
		columns, err := i.inspectColumns(table)
		if err != nil {
			return nil, fmt.Errorf("inspect columns for %s: %w", table, err)
		}
		schema := TableSchema{Name: table, Columns: columns}
		for _, column := range columns {
			if column.PrimaryKey {
				schema.PrimaryKey = column.Name
				break
			}
		}
		schema.Comment, err = i.tableComment(table)
		if err != nil {
			return nil, fmt.Errorf("inspect comment for %s: %w", table, err)
		}
		schema.ForeignKeys, err = i.foreignKeys(table)
		if err != nil {
			return nil, fmt.Errorf("inspect foreign keys for %s: %w", table, err)
		}
		schemas = append(schemas, schema)
	}
	for index := range schemas {
		schemas[index].Relations = relationCandidatesFromSchemas(schemas[index], schemas)
	}
	return schemas, nil
}

func (i *SchemaInspector) inspectColumns(table string) ([]ColumnInfo, error) {
	types, err := i.db.Migrator().ColumnTypes(table)
	if err != nil {
		return nil, err
	}
	primaryKeys, err := i.primaryKeyColumns(table)
	if err != nil {
		return nil, err
	}
	comments, err := i.columnComments(table)
	if err != nil {
		return nil, err
	}
	columns := make([]ColumnInfo, 0, len(types))
	for _, columnType := range types {
		dbType := strings.ToLower(columnType.DatabaseTypeName())
		nullable, _ := columnType.Nullable()
		primaryKey, _ := columnType.PrimaryKey()
		primaryKey = primaryKey || primaryKeys[columnType.Name()]
		defaultValue, _ := columnType.DefaultValue()
		label := columnType.Name()
		if comments[columnType.Name()] != "" {
			label = comments[columnType.Name()]
		}
		columns = append(columns, ColumnInfo{
			Name:         columnType.Name(),
			DBType:       dbType,
			GoType:       goTypeOf(dbType),
			TSType:       tsTypeOf(dbType),
			Nullable:     nullable,
			PrimaryKey:   primaryKey,
			GoField:      exportedName(columnType.Name()),
			Label:        label,
			Comment:      comments[columnType.Name()],
			DefaultValue: defaultValue,
		})
	}
	sort.Slice(columns, func(a, b int) bool { return columns[a].Name < columns[b].Name })
	return columns, nil
}

func (i *SchemaInspector) columnComments(table string) (map[string]string, error) {
	comments := map[string]string{}
	if i.db.Dialector.Name() != "postgres" {
		return comments, nil
	}
	type columnComment struct {
		ColumnName string `gorm:"column:column_name"`
		Comment    string `gorm:"column:comment"`
	}
	var rows []columnComment
	const query = `SELECT a.attname AS column_name, COALESCE(col_description(c.oid, a.attnum), '') AS comment
		FROM pg_class c
		JOIN pg_attribute a ON a.attrelid = c.oid
		WHERE c.oid = to_regclass(?) AND a.attnum > 0 AND NOT a.attisdropped`
	if err := i.db.Raw(query, table).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		comments[row.ColumnName] = row.Comment
	}
	return comments, nil
}

func (i *SchemaInspector) primaryKeyColumns(table string) (map[string]bool, error) {
	result := map[string]bool{}
	switch i.db.Dialector.Name() {
	case "sqlite":
		type sqliteColumn struct {
			Name string `gorm:"column:name"`
			PK   int    `gorm:"column:pk"`
		}
		var rows []sqliteColumn
		quoted := strings.ReplaceAll(table, `"`, `""`)
		if err := i.db.Raw(fmt.Sprintf(`PRAGMA table_info("%s")`, quoted)).Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if row.PK > 0 {
				result[row.Name] = true
			}
		}
	case "postgres":
		var names []string
		err := i.db.Raw(`
			SELECT kcu.column_name
			FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage kcu
			  ON tc.constraint_name = kcu.constraint_name AND tc.constraint_schema = kcu.constraint_schema
			WHERE tc.constraint_type = 'PRIMARY KEY'
			  AND tc.table_schema = current_schema()
			  AND tc.table_name = ?`, table).Scan(&names).Error
		if err != nil {
			return nil, err
		}
		for _, name := range names {
			result[name] = true
		}
	}
	return result, nil
}

func (i *SchemaInspector) tableComment(table string) (string, error) {
	if i.db.Dialector.Name() != "postgres" {
		return "", nil
	}
	var comment string
	err := i.db.Raw(`SELECT COALESCE(obj_description(to_regclass(?), 'pg_class'), '')`, table).Scan(&comment).Error
	return comment, err
}

func (i *SchemaInspector) foreignKeys(table string) ([]ForeignKeyInfo, error) {
	if i.db.Dialector.Name() == "postgres" {
		return i.postgresForeignKeys(table)
	}
	if i.db.Dialector.Name() == "sqlite" {
		return i.sqliteForeignKeys(table)
	}
	return nil, nil
}

func (i *SchemaInspector) sqliteForeignKeys(table string) ([]ForeignKeyInfo, error) {
	type sqliteForeignKey struct {
		ID    int    `gorm:"column:id"`
		Seq   int    `gorm:"column:seq"`
		Table string `gorm:"column:table"`
		From  string `gorm:"column:from"`
		To    string `gorm:"column:to"`
	}
	var rows []sqliteForeignKey
	quoted := strings.ReplaceAll(table, `"`, `""`)
	if err := i.db.Raw(fmt.Sprintf(`PRAGMA foreign_key_list("%s")`, quoted)).Scan(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]ForeignKeyInfo, 0, len(rows))
	for _, row := range rows {
		result = append(result, ForeignKeyInfo{
			Name:        fmt.Sprintf("fk_%s_%d_%d", table, row.ID, row.Seq),
			Field:       row.From,
			TargetTable: row.Table,
			TargetField: row.To,
		})
	}
	sortForeignKeys(result)
	return result, nil
}

func (i *SchemaInspector) postgresForeignKeys(table string) ([]ForeignKeyInfo, error) {
	var result []ForeignKeyInfo
	err := i.db.Raw(`
		SELECT tc.constraint_name AS name,
		       kcu.column_name AS field,
		       ccu.table_name AS target_table,
		       ccu.column_name AS target_field
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON tc.constraint_name = kcu.constraint_name AND tc.constraint_schema = kcu.constraint_schema
		JOIN information_schema.constraint_column_usage ccu
		  ON ccu.constraint_name = tc.constraint_name AND ccu.constraint_schema = tc.constraint_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
		  AND tc.table_schema = current_schema()
		  AND tc.table_name = ?`, table).Scan(&result).Error
	sortForeignKeys(result)
	return result, err
}

func relationCandidatesFromSchemas(source TableSchema, schemas []TableSchema) []RelationCandidate {
	result := make([]RelationCandidate, 0, len(source.ForeignKeys))
	for _, foreignKey := range source.ForeignKeys {
		result = append(result, RelationCandidate{
			Kind:        RelationManyToOne,
			SourceTable: source.Name,
			TargetTable: foreignKey.TargetTable,
			FKField:     foreignKey.Field,
		})
	}

	for _, candidate := range schemas {
		if candidate.Name == source.Name {
			continue
		}
		for _, foreignKey := range candidate.ForeignKeys {
			if foreignKey.TargetTable == source.Name {
				result = append(result, RelationCandidate{
					Kind:        RelationOneToMany,
					SourceTable: source.Name,
					TargetTable: candidate.Name,
					FKField:     foreignKey.Field,
				})
			}
		}
		if len(candidate.ForeignKeys) != 2 || hasJoinPayload(candidate.Columns, candidate.ForeignKeys) {
			continue
		}
		for index, own := range candidate.ForeignKeys {
			if own.TargetTable != source.Name {
				continue
			}
			other := candidate.ForeignKeys[1-index]
			result = append(result, RelationCandidate{
				Kind:        RelationManyToMany,
				SourceTable: source.Name,
				TargetTable: other.TargetTable,
				JoinTable:   candidate.Name,
				FKField:     own.Field,
				TargetFK:    other.Field,
			})
		}
	}
	sort.Slice(result, func(a, b int) bool {
		left := relationSortKey(result[a])
		right := relationSortKey(result[b])
		return left < right
	})
	return result
}

func relationSortKey(value RelationCandidate) string {
	return strings.Join([]string{
		string(value.Kind), value.SourceTable, value.TargetTable, value.JoinTable,
		value.FKField, value.TargetFK,
	}, "\x00")
}

func hasJoinPayload(columns []ColumnInfo, foreignKeys []ForeignKeyInfo) bool {
	foreignKeyFields := make(map[string]bool, len(foreignKeys))
	for _, foreignKey := range foreignKeys {
		foreignKeyFields[foreignKey.Field] = true
	}
	for _, column := range columns {
		if column.PrimaryKey || foreignKeyFields[column.Name] || isServerManagedColumn(column.Name) {
			continue
		}
		return true
	}
	return false
}

func sortForeignKeys(values []ForeignKeyInfo) {
	sort.Slice(values, func(a, b int) bool {
		left := strings.Join([]string{
			values[a].Field, values[a].TargetTable, values[a].TargetField, values[a].Name,
		}, "\x00")
		right := strings.Join([]string{
			values[b].Field, values[b].TargetTable, values[b].TargetField, values[b].Name,
		}, "\x00")
		return left < right
	})
}
