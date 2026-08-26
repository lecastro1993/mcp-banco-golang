# MCP banco local

Servidor MCP local para consulta SQL em bancos `Postgres` e `MySQL`.

## Pré-requisitos

- Go 1.24 ou superior instalado
- Banco acessível na máquina local ou na rede
- Variáveis de ambiente configuradas em `.env` ou no ambiente do sistema

### Instalação do Go

Verifique primeiro se o Go já está disponível:

```bash
go version
```

Se o comando não existir, instale o Go pela página oficial:

```text
https://go.dev/dl/
```

No Linux, uma forma comum é extrair o pacote em `/usr/local` e adicionar o binário ao `PATH`:

```bash
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf go1.24.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

Depois, valide a instalação:

```bash
go version
```

## Variáveis de ambiente

O projeto lê as variáveis abaixo na inicialização:

```env
DB_TYPE=postgres
DB_HOST=localhost
DB_PORT=5432
DB_NAME=meu_banco
DB_USER=usuario
DB_PASSWORD=senha
DB_SSLMODE=disable
```

Também é possível usar uma DSN completa:

```env
DB_TYPE=postgres
DB_DSN=postgres://usuario:senha@localhost:5432/seu_banco?sslmode=disable
```

Exemplo para MySQL:

```env
DB_TYPE=mysql
DB_HOST=localhost
DB_PORT=3306
DB_NAME=meu_banco
DB_USER=usuario
DB_PASSWORD=senha
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

Regras de seleção do banco:

- se `DB_TYPE` estiver definido, ele define o driver
- se `DB_TYPE` não estiver definido, o projeto tenta inferir o driver a partir de `DB_DSN`

## Executar localmente

1. Copie o arquivo de exemplo para `.env` e ajuste os dados do banco:

```bash
cp .env-exemple .env
```

2. Edite o `.env` com os dados corretos do Postgres ou MySQL.

3. Baixe as dependências:

```bash
go mod download
```

4. Execute o servidor em modo desenvolvimento:

```bash
go run .
```

Ou gere o binário local:

```bash
go build -o banco-mcp .
./banco-mcp
```

O servidor roda em `stdio`, então ele deve ser iniciado por um cliente MCP.

Se tudo estiver correto, o processo vai permanecer ativo aguardando conexões do cliente MCP.

## Configuração no cliente MCP

Exemplo simples de configuração do servidor local. Ajuste o path para o diretório do seu workspace na máquina:

```json
{
  "mcpServers": {
    "banco": {
      "command": "~/www/mcp-banco-golang/banco-mcp",
      "cwd": "~/www/mcp-banco-golang"
    }
  }
}
```

Se o seu workspace estiver em outro diretório, substitua o path acima pelo caminho local correspondente.

Se preferir não gerar binário:

```json
{
  "mcpServers": {
    "banco": {
      "command": "go",
      "args": ["run", "."],
      "cwd": "~/www/mcp-banco-golang"
    }
  }
}
```

## Ferramenta disponível

O MCP registra a ferramenta:

- `query`: executa uma consulta SQL no banco configurado e retorna colunas e linhas em JSON
- `list_tables`: lista as tabelas visíveis no banco configurado

Exemplo de uso:

```json
{
  "sql": "select 1 as ok"
}
```

Exemplo de uso com MySQL:

```json
{
  "sql": "select now() as server_time"
}
```

Exemplo de uso para listar tabelas:

```json
{}
```

Resposta esperada:

```json
{
  "tables": ["clientes", "pedidos"],
  "rowCount": 2
}
```

## Observações

- O arquivo `.env` é carregado automaticamente se existir
- Se `DB_DSN` não for informada, a conexão é montada a partir das demais variáveis
- Se a configuração estiver incompleta, o processo encerra com erro na inicialização
