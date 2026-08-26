package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type App struct {
	driver   string
	postgres *pgxpool.Pool
	mysql    *sql.DB
	logger   *log.Logger
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
	SQL string `json:"sql"`
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

func (app *App) registerTools(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "query",
		Description: fmt.Sprintf("Executa uma consulta SQL no banco configurado (%s) e retorna colunas e linhas em JSON.", app.driver),
	}, app.queryTool)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_tables",
		Description: fmt.Sprintf("Lista as tabelas do banco configurado (%s).", app.driver),
	}, app.listTablesTool)
}

func (app *App) queryTool(ctx context.Context, _ *mcp.CallToolRequest, input QueryInput) (*mcp.CallToolResult, QueryOutput, error) {
	switch app.driver {
	case "postgres":
		return queryPostgres(ctx, app.postgres, input)
	case "mysql":
		return queryMySQL(ctx, app.mysql, input)
	default:
		return nil, QueryOutput{}, fmt.Errorf("driver não suportado: %s", app.driver)
	}
}

func (app *App) listTablesTool(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, TableListOutput, error) {
	switch app.driver {
	case "postgres":
		return listTablesPostgres(ctx, app.postgres)
	case "mysql":
		return listTablesMySQL(ctx, app.mysql)
	default:
		return nil, TableListOutput{}, fmt.Errorf("driver não suportado: %s", app.driver)
	}
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

	cfg := getDatabaseConfig()
	dsn := cfg.buildDSN()
	if cfg.Driver == "" || dsn == "" {
		logger.Fatal("configuração de banco inválida; use DB_TYPE=postgres|mysql e DB_HOST/DB_PORT/DB_NAME/DB_USER/DB_PASSWORD ou DB_DSN")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	app := &App{
		driver: cfg.Driver,
		logger: logger,
	}

	switch cfg.Driver {
	case "postgres":
		db, err := pgxpool.New(ctx, dsn)
		if err != nil {
			logger.Fatalf("erro ao abrir pool Postgres: %v", err)
		}

		if err := db.Ping(ctx); err != nil {
			logger.Fatalf("erro ao conectar no Postgres: %v", err)
		}

		app.postgres = db
		logger.Println("Conexão com Postgres realizada com sucesso")
	case "mysql":
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			logger.Fatalf("erro ao abrir conexão MySQL: %v", err)
		}

		db.SetConnMaxLifetime(time.Minute * 5)
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(10)

		if err := db.PingContext(ctx); err != nil {
			logger.Fatalf("erro ao conectar no MySQL: %v", err)
		}

		app.mysql = db
		logger.Println("Conexão com MySQL realizada com sucesso")
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-banco",
		Version: "1.0.0",
	}, nil)

	app.registerTools(server)

	logger.Printf("MCP iniciado (driver=%s)", app.driver)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		logger.Fatalf("server failed: %v", err)
	}
}
