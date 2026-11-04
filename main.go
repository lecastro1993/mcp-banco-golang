package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type App struct {
	connections     map[string]*DatabaseConnection
	defaultDatabase string
	logger          *log.Logger
}

type DatabaseConnection struct {
	Name     string        `json:"name"`
	Driver   string        `json:"driver"`
	DSN      string        `json:"dsn"`
	Host     string        `json:"host"`
	Port     string        `json:"port"`
	Database string        `json:"database"`
	User     string        `json:"user"`
	Password string        `json:"password"`
	SSLMode  string        `json:"sslmode"`
	Postgres *pgxpool.Pool `json:"-"`
	MySQL    *sql.DB       `json:"-"`
}

type DatabaseConfig struct {
	Driver   string
	DSN      string
	Host     string
	Port     string
	Database string
	User     string
	Password string
	SSLMode  string
}

type QueryInput struct {
	SQL      string `json:"sql"`
	Database string `json:"database,omitempty"`
}

type DatabaseListOutput struct {
	Databases       []string `json:"databases"`
	DefaultDatabase string   `json:"defaultDatabase"`
}

type QueryOutput struct {
	Columns  []string         `json:"columns"`
	Rows     []map[string]any `json:"rows"`
	RowCount int              `json:"rowCount"`
}

type TableListOutput struct {
	Tables   []string `json:"tables"`
	RowCount int      `json:"rowCount"`
}

func loadEnv(logger *log.Logger) {
	if err := godotenv.Load(); err != nil {
		logger.Println(".env não encontrado, usando variáveis do ambiente do sistema")
	}
}

func getDatabaseConfig() DatabaseConfig {
	cfg := DatabaseConfig{
		Driver:   strings.ToLower(strings.TrimSpace(os.Getenv("DB_TYPE"))),
		DSN:      strings.TrimSpace(os.Getenv("DB_DSN")),
		Host:     strings.TrimSpace(os.Getenv("DB_HOST")),
		Port:     strings.TrimSpace(os.Getenv("DB_PORT")),
		Database: strings.TrimSpace(os.Getenv("DB_NAME")),
		User:     strings.TrimSpace(os.Getenv("DB_USER")),
		Password: os.Getenv("DB_PASSWORD"),
		SSLMode:  strings.TrimSpace(os.Getenv("DB_SSLMODE")),
	}

	if cfg.SSLMode == "" {
		cfg.SSLMode = "disable"
	}

	if cfg.Driver == "" {
		cfg.Driver = inferDriverFromDSN(cfg.DSN)
	}

	return cfg
}

func getDatabaseConfigs() ([]DatabaseConnection, error) {
	raw := strings.TrimSpace(os.Getenv("DB_CONNECTIONS"))
	if raw == "" {
		legacy := getDatabaseConfig()
		return []DatabaseConnection{{
			Name: "default", Driver: legacy.Driver, DSN: legacy.DSN,
			Host: legacy.Host, Port: legacy.Port, Database: legacy.Database,
			User: legacy.User, Password: legacy.Password, SSLMode: legacy.SSLMode,
		}}, nil
	}

	var configs []DatabaseConnection
	if err := json.Unmarshal([]byte(raw), &configs); err != nil {
		return nil, fmt.Errorf("DB_CONNECTIONS inválida: %w", err)
	}
	if len(configs) == 0 {
		return nil, fmt.Errorf("DB_CONNECTIONS deve conter pelo menos uma conexão")
	}
	for i := range configs {
		configs[i].Name = strings.TrimSpace(configs[i].Name)
		configs[i].Driver = strings.ToLower(strings.TrimSpace(configs[i].Driver))
		configs[i].SSLMode = strings.TrimSpace(configs[i].SSLMode)
		if configs[i].SSLMode == "" {
			configs[i].SSLMode = "disable"
		}
		if configs[i].Driver == "" {
			configs[i].Driver = inferDriverFromDSN(configs[i].DSN)
		}
		if configs[i].Name == "" {
			return nil, fmt.Errorf("conexão na posição %d não tem name", i)
		}
	}
	return configs, nil
}

func inferDriverFromDSN(dsn string) string {
	switch {
	case strings.HasPrefix(dsn, "postgres://"), strings.HasPrefix(dsn, "postgresql://"):
		return "postgres"
	case strings.Contains(dsn, "@tcp("):
		return "mysql"
	default:
		return ""
	}
}

func (cfg DatabaseConfig) buildDSN() string {
	if cfg.DSN != "" {
		return cfg.DSN
	}

	if cfg.Host == "" || cfg.Port == "" || cfg.Database == "" || cfg.User == "" {
		return ""
	}

	switch cfg.Driver {
	case "postgres":
		return fmt.Sprintf(
			"postgres://%s:%s@%s:%s/%s?sslmode=%s",
			cfg.User,
			cfg.Password,
			cfg.Host,
			cfg.Port,
			cfg.Database,
			cfg.SSLMode,
		)
	case "mysql":
		return fmt.Sprintf(
			"%s:%s@tcp(%s:%s)/%s?parseTime=true",
			cfg.User,
			cfg.Password,
			cfg.Host,
			cfg.Port,
			cfg.Database,
		)
	default:
		return ""
	}
}

func (cfg DatabaseConnection) buildDSN() string {
	return (DatabaseConfig{
		Driver: cfg.Driver, DSN: cfg.DSN, Host: cfg.Host, Port: cfg.Port,
		Database: cfg.Database, User: cfg.User, Password: cfg.Password, SSLMode: cfg.SSLMode,
	}).buildDSN()
}

func (app *App) registerTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "query",
		Description: "Executa uma consulta SQL em um banco conectado. Informe database ou usa o banco padrão.",
	}, app.queryTool)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tables",
		Description: "Lista as tabelas de um banco conectado. Informe database ou usa o banco padrão.",
	}, app.listTablesTool)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_databases",
		Description: "Lista os bancos conectados e informa qual é o banco padrão.",
	}, app.listDatabasesTool)
}

func (app *App) connection(name string) (*DatabaseConnection, error) {
	if name == "" {
		name = app.defaultDatabase
	}
	connection, ok := app.connections[name]
	if !ok {
		return nil, fmt.Errorf("banco %q não encontrado; use list_databases", name)
	}
	return connection, nil
}

func (app *App) queryTool(ctx context.Context, _ *mcp.CallToolRequest, input QueryInput) (*mcp.CallToolResult, QueryOutput, error) {
	connection, err := app.connection(input.Database)
	if err != nil {
		return nil, QueryOutput{}, err
	}
	switch connection.Driver {
	case "postgres":
		return queryPostgres(ctx, connection.Postgres, input)
	case "mysql":
		return queryMySQL(ctx, connection.MySQL, input)
	default:
		return nil, QueryOutput{}, fmt.Errorf("driver não suportado: %s", connection.Driver)
	}
}

type ListTablesInput struct {
	Database string `json:"database,omitempty"`
}

func (app *App) listTablesTool(ctx context.Context, _ *mcp.CallToolRequest, input ListTablesInput) (*mcp.CallToolResult, TableListOutput, error) {
	connection, err := app.connection(input.Database)
	if err != nil {
		return nil, TableListOutput{}, err
	}
	switch connection.Driver {
	case "postgres":
		return listTablesPostgres(ctx, connection.Postgres)
	case "mysql":
		return listTablesMySQL(ctx, connection.MySQL)
	default:
		return nil, TableListOutput{}, fmt.Errorf("driver não suportado: %s", connection.Driver)
	}
}

func (app *App) listDatabasesTool(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, DatabaseListOutput, error) {
	databases := make([]string, 0, len(app.connections))
	for name := range app.connections {
		databases = append(databases, name)
	}
	sort.Strings(databases)
	return nil, DatabaseListOutput{Databases: databases, DefaultDatabase: app.defaultDatabase}, nil
}

func queryPostgres(ctx context.Context, db *pgxpool.Pool, input QueryInput) (*mcp.CallToolResult, QueryOutput, error) {
	if db == nil {
		return nil, QueryOutput{}, fmt.Errorf("conexão Postgres não configurada")
	}

	rows, err := db.Query(ctx, input.SQL)
	if err != nil {
		return nil, QueryOutput{}, err
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	columns := make([]string, len(fields))
	for i, field := range fields {
		columns[i] = field.Name
	}

	result := QueryOutput{
		Columns: columns,
		Rows:    make([]map[string]any, 0),
	}

	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, QueryOutput{}, err
		}

		row := make(map[string]any, len(columns))
		for i, col := range columns {
			row[col] = values[i]
		}

		result.Rows = append(result.Rows, row)
	}

	if err := rows.Err(); err != nil {
		return nil, QueryOutput{}, err
	}

	result.RowCount = len(result.Rows)
	return nil, result, nil
}

func listTablesPostgres(ctx context.Context, db *pgxpool.Pool) (*mcp.CallToolResult, TableListOutput, error) {
	if db == nil {
		return nil, TableListOutput{}, fmt.Errorf("conexão Postgres não configurada")
	}

	const sqlText = `
		select tablename
		from pg_catalog.pg_tables
		where schemaname not in ('pg_catalog', 'information_schema')
		order by tablename
	`

	rows, err := db.Query(ctx, sqlText)
	if err != nil {
		return nil, TableListOutput{}, err
	}
	defer rows.Close()

	tables := make([]string, 0)
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, TableListOutput{}, err
		}
		tables = append(tables, tableName)
	}

	if err := rows.Err(); err != nil {
		return nil, TableListOutput{}, err
	}

	result := TableListOutput{Tables: tables, RowCount: len(tables)}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: formatTableList(result)}},
	}, result, nil
}

func queryMySQL(ctx context.Context, db *sql.DB, input QueryInput) (*mcp.CallToolResult, QueryOutput, error) {
	if db == nil {
		return nil, QueryOutput{}, fmt.Errorf("conexão MySQL não configurada")
	}

	rows, err := db.QueryContext(ctx, input.SQL)
	if err != nil {
		return nil, QueryOutput{}, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, QueryOutput{}, err
	}

	result := QueryOutput{
		Columns: columns,
		Rows:    make([]map[string]any, 0),
	}

	for rows.Next() {
		row := make(map[string]any, len(columns))
		values := make([]any, len(columns))
		valuePointers := make([]any, len(columns))
		for i := range values {
			valuePointers[i] = &values[i]
		}

		if err := rows.Scan(valuePointers...); err != nil {
			return nil, QueryOutput{}, err
		}

		for i, col := range columns {
			row[col] = normalizeSQLValue(values[i])
		}

		result.Rows = append(result.Rows, row)
	}

	if err := rows.Err(); err != nil {
		return nil, QueryOutput{}, err
	}

	result.RowCount = len(result.Rows)
	return nil, result, nil
}

func listTablesMySQL(ctx context.Context, db *sql.DB) (*mcp.CallToolResult, TableListOutput, error) {
	if db == nil {
		return nil, TableListOutput{}, fmt.Errorf("conexão MySQL não configurada")
	}

	rows, err := db.QueryContext(ctx, "show tables")
	if err != nil {
		return nil, TableListOutput{}, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, TableListOutput{}, err
	}
	if len(columns) == 0 {
		return nil, TableListOutput{Tables: []string{}, RowCount: 0}, nil
	}

	tables := make([]string, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		valuePointers := make([]any, len(columns))
		for i := range values {
			valuePointers[i] = &values[i]
		}

		if err := rows.Scan(valuePointers...); err != nil {
			return nil, TableListOutput{}, err
		}

		for _, value := range values {
			tableName, ok := normalizeSQLValue(value).(string)
			if ok {
				tables = append(tables, tableName)
				break
			}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, TableListOutput{}, err
	}

	result := TableListOutput{Tables: tables, RowCount: len(tables)}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: formatTableList(result)}},
	}, result, nil
}

func formatTableList(result TableListOutput) string {
	if result.RowCount == 0 {
		return "Nenhuma tabela encontrada."
	}

	return fmt.Sprintf("Tabelas encontradas (%d): %s", result.RowCount, strings.Join(result.Tables, ", "))
}

func normalizeSQLValue(value any) any {
	if bytes, ok := value.([]byte); ok {
		return string(bytes)
	}

	return value
}

func main() {
	logger := log.New(os.Stderr, "[mcp-banco] ", log.LstdFlags|log.Lshortfile)

	loadEnv(logger)

	configs, err := getDatabaseConfigs()
	if err != nil {
		logger.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	app := &App{connections: make(map[string]*DatabaseConnection), logger: logger}
	defaultDatabase := strings.TrimSpace(os.Getenv("DB_DEFAULT"))
	for i, cfg := range configs {
		if cfg.Name == "" || cfg.Driver == "" || cfg.buildDSN() == "" {
			logger.Fatalf("configuração inválida para a conexão %d (%q)", i, cfg.Name)
		}
		if _, exists := app.connections[cfg.Name]; exists {
			logger.Fatalf("nome de conexão duplicado: %s", cfg.Name)
		}

		connection := cfg
		switch cfg.Driver {
		case "postgres":
			db, err := pgxpool.New(ctx, cfg.buildDSN())
			if err != nil {
				logger.Fatalf("erro ao abrir pool Postgres %q: %v", cfg.Name, err)
			}
			if err := db.Ping(ctx); err != nil {
				logger.Fatalf("erro ao conectar no Postgres %q: %v", cfg.Name, err)
			}
			connection.Postgres = db
		case "mysql":
			db, err := sql.Open("mysql", cfg.buildDSN())
			if err != nil {
				logger.Fatalf("erro ao abrir conexão MySQL %q: %v", cfg.Name, err)
			}
			db.SetConnMaxLifetime(time.Minute * 5)
			db.SetMaxOpenConns(10)
			db.SetMaxIdleConns(10)
			if err := db.PingContext(ctx); err != nil {
				logger.Fatalf("erro ao conectar no MySQL %q: %v", cfg.Name, err)
			}
			connection.MySQL = db
		default:
			logger.Fatalf("driver não suportado na conexão %q: %s", cfg.Name, cfg.Driver)
		}
		app.connections[cfg.Name] = &connection
		logger.Printf("Conexão %q (%s) realizada com sucesso", cfg.Name, cfg.Driver)
	}
	if defaultDatabase == "" {
		defaultDatabase = configs[0].Name
	}
	if _, ok := app.connections[defaultDatabase]; !ok {
		logger.Fatalf("DB_DEFAULT aponta para uma conexão inexistente: %s", defaultDatabase)
	}
	app.defaultDatabase = defaultDatabase

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-banco",
		Version: "1.0.0",
	}, nil)

	app.registerTools(server)

	logger.Printf("MCP iniciado (bancos=%d, padrão=%s)", len(app.connections), app.defaultDatabase)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		logger.Fatalf("server failed: %v", err)
	}
}
