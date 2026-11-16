# 🔌 API REST

> 🗣️ **Para que serve isto?** O app no navegador já usa esta API por trás. **Você não precisa dela para usar o sistema.** Ela serve para quem quiser automatizar (um script, outro sistema) ou testar pelo terminal com `curl`.

Base: `http://localhost:8080` (local) ou `https://lcr.d4ffy.com` (produção).

🍪 A autenticação é por **cookie de sessão** (`HttpOnly; SameSite=Strict`), criado no login. Erros voltam como JSON `{"erro": "mensagem"}`.

## 🔓 Públicas

| Método | Rota | Corpo | Descrição |
|---|---|---|---|
| `POST` | `/api/cadastro` | JSON `{nome, cpf, senha}` | 🪪 cria a conta e a AC emite o certificado |
| `POST` | `/api/login` | JSON `{cpf, senha}` | 🔑 abre a sessão (cookie) |
| `POST` | `/api/logout` | — | 🚪 encerra a sessão |
| `POST` | `/api/validar` | multipart `arquivo` | 🔍 valida o PDF e devolve o resultado em JSON |
| `GET` | `/api/ac` | — | 🏛️ dados da AC Raiz e da Intermediária |
| `GET` | `/api/ac/raiz.crt` | — | 📥 certificado da AC Raiz (PEM) |
| `GET` | `/api/ac/intermediaria.crt` | — | 📥 certificado da AC Intermediária (PEM) |
| `GET` | `/api/ac/lcr.crl` | — | 📜 LCR atual (DER) |

## 🔒 Autenticadas

| Método | Rota | Corpo | Descrição |
|---|---|---|---|
| `GET` | `/api/eu` | — | 👤 usuário logado e o certificado dele |
| `POST` | `/api/assinar` | multipart: `arquivo`, `pagina`, `x1`, `y1`, `x2`, `y2` (pontos PDF), `senha`, `motivo` | 🖋️ assina e devolve o PDF |
| `GET` | `/api/documentos` | — | 📂 documentos que você assinou (inclui os assinados também por outras pessoas), com a lista `signatarios` |
| `POST` | `/api/documentos/{id}` | JSON `{senha}` | 📥 decifra e baixa o PDF. Só para quem assinou; a senha libera a chave privada. `403` senha errada, `404` sem acesso, `422` conteúdo adulterado |
| `GET` | `/api/certificado.crt` | — | 📥 certificado do usuário (PEM) |
| `POST` | `/api/certificado/pfx` | JSON `{senha}` | 💾 PKCS#12 (AES-256-CBC + PBKDF2-SHA256) |
| `POST` | `/api/certificado/revogar` | JSON `{senha, motivo}` | 🚫 revoga (motivo = código CRLReason: 0, 1, 3, 4, 5) |
| `POST` | `/api/certificado/renovar` | JSON `{senha}` | 🔄 novo par de chaves e novo certificado |

## 🛡️ Respostas de proteção

| Status | Quando |
|---|---|
| `403` | senha incorreta, ou requisição de **outra origem** (CSRF) |
| `429` | limite de tentativas estourado; o cabeçalho `Retry-After` diz quantos segundos esperar |
| `503` | servidor ocupado (fila de PDFs cheia por mais de 30 s) |

Os limites estão em [SEGURANCA.md](SEGURANCA.md#forca-bruta).

📏 PDFs: até **20 MiB**, e o arquivo precisa começar com `%PDF-`.

## 🧪 Exemplos com `curl`

Fluxo completo pelo terminal, equivalente ao que se faz clicando no app. Os comandos rodam em sequência, na mesma pasta, com um `contrato.pdf` qualquer nela. O arquivo `cookies.txt` guarda a sessão entre um comando e outro.

```bash
# 🏛️ baixar a cadeia da AC
curl -s localhost:8080/api/ac/raiz.crt -o raiz.crt
curl -s localhost:8080/api/ac/intermediaria.crt -o inter.crt

# 🪪 criar conta e logar (guardando o cookie)
curl -s -c cookies.txt localhost:8080/api/cadastro -H 'Content-Type: application/json' \
  -d '{"nome":"Maria Silva","cpf":"529.982.247-25","senha":"senha-forte-123"}'
curl -s -c cookies.txt localhost:8080/api/login -H 'Content-Type: application/json' \
  -d '{"cpf":"529.982.247-25","senha":"senha-forte-123"}'

# 🖋️ assinar a página 1, carimbo no canto inferior esquerdo
curl -s -b cookies.txt localhost:8080/api/assinar \
  -F arquivo=@contrato.pdf -F pagina=1 -F x1=40 -F y1=40 -F x2=260 -F y2=110 \
  -F senha=senha-forte-123 -F motivo="Aprovação" -o contrato_assinado.pdf

# 🔍 validar
curl -s localhost:8080/api/validar -F arquivo=@contrato_assinado.pdf
```
