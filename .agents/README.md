# Configuração de múltiplos bancos

Este guia explica como configurar o servidor MCP para conectar mais de um banco de dados no mesmo processo.

## 1. Configuração recomendada

Use a variável `DB_CONNECTIONS` com um array JSON. Cada item representa uma conexão e precisa de um `name` exclusivo.

```env
DB_DEFAULT=principal
DB_CONNECTIONS=[{"name":"principal","driver":"postgres","host":"localhost","port":"5432","database":"vendas","user":"postgres","password":"senha","sslmode":"disable"},{"name":"relatorios","driver":"mysql","host":"localhost","port":"3306","database":"relatorios","user":"root","password":"senha"}]
```

`DB_DEFAULT` é opcional. Quando não informado, a primeira conexão do array será usada como padrão.

## 2. Usando DSN

Em vez de informar host, porta, banco e usuário separadamente, cada conexão pode usar `dsn`:

```env
DB_DEFAULT=producao
DB_CONNECTIONS=[{"name":"producao","driver":"postgres","dsn":"postgres://usuario:senha@localhost:5432/producao?sslmode=disable"},{"name":"legado","driver":"mysql","dsn":"root:senha@tcp(localhost:3306)/legado?parseTime=true"}]
```

Os drivers disponíveis são `postgres` e `mysql`. O campo `driver` pode ser omitido quando o servidor conseguir inferi-lo pela DSN.

## 3. Configuração no cliente MCP

No cliente MCP, passe as variáveis no bloco de ambiente do servidor:

```json
{
  "mcp": {
    "banco": {
      "type": "local",
      "command": ["go", "run", "."],
      "cwd": "/caminho/para/mcp-banco",
      "environment": {
        "DB_DEFAULT": "principal",
        "DB_CONNECTIONS": "[{\"name\":\"principal\",\"driver\":\"postgres\",\"dsn\":\"postgres://usuario:senha@localhost:5432/vendas?sslmode=disable\"},{\"name\":\"relatorios\",\"driver\":\"mysql\",\"dsn\":\"root:senha@tcp(localhost:3306)/relatorios?parseTime=true\"}]"
      }
    }
  }
}
```

Se usar `.env`, coloque essas mesmas variáveis no arquivo e inicie o servidor normalmente:

```bash
go run .
```

## 4. Selecionando um banco nas ferramentas

Primeiro, use `list_databases` para conferir os nomes disponíveis:

```json
{}
```

Depois, informe `database` nas ferramentas que devem usar uma conexão específica:

```json
{
  "database": "relatorios",
  "sql": "select now() as horario"
}
```

O campo `database` é aceito por:

- `query`: executa SQL na conexão selecionada;
- `list_tables`: lista as tabelas da conexão selecionada.

Se `database` não for informado, o servidor usa `DB_DEFAULT`.

## 5. Configuração legada de um único banco

A configuração antiga continua funcionando:

```env
DB_TYPE=postgres
DB_HOST=localhost
DB_PORT=5432
DB_NAME=vendas
DB_USER=postgres
DB_PASSWORD=senha
DB_SSLMODE=disable
```

Quando `DB_CONNECTIONS` estiver definida, ela terá prioridade sobre as variáveis `DB_TYPE`, `DB_HOST`, `DB_PORT`, `DB_NAME`, `DB_USER`, `DB_PASSWORD` e `DB_DSN`.

## 6. Boas práticas

- Não coloque senhas reais em arquivos versionados.
- Mantenha `.env` e `opencode.json` fora do Git.
- Prefira DSNs com SSL habilitado em bancos remotos.
- Use nomes claros, como `producao`, `homologacao`, `relatorios` e `legado`.
- Confirme se todos os bancos estão acessíveis antes de iniciar o servidor: ele testa cada conexão durante a inicialização.

