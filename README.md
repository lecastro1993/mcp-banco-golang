# MCP banco local

Configuração do MCP de banco na sua máquina, dentro de `/home/reed/www/workspace/mcp-banco`.

## Pré-requisitos

- Go instalado
- Banco acessível na máquina local ou na rede
- Variáveis de ambiente configuradas em `.env` ou no ambiente do sistema

## Variáveis de ambiente

O projeto lê as variáveis abaixo na inicialização:

```env
DB_TYPE=postgres
DB_HOST=localhost
DB_PORT=5432
DB_NAME=sete
DB_USER=root
DB_PASSWORD=123123
DB_SSLMODE=disable
```

Também é possível usar uma DSN completa:

```env
DB_TYPE=postgres
DB_DSN=postgres://usuario:senha@localhost:5432/seu_banco?sslmode=disable
```

Campos suportados:

- `DB_TYPE`: `postgres` ou `mysql`
- `DB_DSN`: string de conexão completa; se estiver definida, tem prioridade
- `DB_HOST`: host do banco
- `DB_PORT`: porta do banco
- `DB_NAME`: nome do banco
- `DB_USER`: usuário
- `DB_PASSWORD`: senha
- `DB_SSLMODE`: usado no Postgres; padrão `disable`

## Executar localmente

Para rodar em desenvolvimento:

```bash
go run .
```

Para gerar o binário local:

```bash
go build -o banco-mcp .
```

Para executar o binário:

```bash
./banco-mcp
```

O servidor roda em `stdio`, então ele deve ser iniciado por um cliente MCP.

## Configuração no cliente MCP

Exemplo simples de configuração do servidor local:

```json
{
  "mcpServers": {
    "banco": {
      "command": "/home/reed/www/workspace/mcp-banco/banco-mcp",
      "cwd": "/home/reed/www/workspace/mcp-banco"
    }
  }
}
```

Se preferir não gerar binário:

```json
{
  "mcpServers": {
    "banco": {
      "command": "go",
      "args": ["run", "."],
      "cwd": "/home/reed/www/workspace/mcp-banco"
    }
  }
}
```

## Ferramenta disponível

O MCP registra a ferramenta:

- `query`: executa uma consulta SQL no banco configurado e retorna colunas e linhas em JSON

Exemplo de uso:

```json
{
  "sql": "select 1 as ok"
}
```

## Observações

- O arquivo `.env` é carregado automaticamente se existir
- Se `DB_DSN` não for informada, a conexão é montada a partir das demais variáveis
- Se a configuração estiver incompleta, o processo encerra com erro na inicialização
