# 🛡️ Segurança

> 🗣️ **Em palavras simples:** além da criptografia dos certificados e dos documentos, a aplicação tem defesas contra os ataques comuns a qualquer site: adivinhar senhas, enganar o navegador de quem está logado, enviar arquivos maliciosos, espiar dados de outro usuário e abusar do servidor. Este documento lista **cada ataque**, **o que o impede** e **como testar**.

📑 **Sumário**

1. [🗺️ Visão geral](#visao)
2. [🚪 Força bruta e abuso](#forca-bruta)
3. [🎭 CSRF e subdomínios irmãos](#csrf)
4. [🧱 Cabeçalhos do navegador (XSS, clickjacking)](#cabecalhos)
5. [🍪 Sessão e cookie](#sessao)
6. [🧪 Validação de entradas e uploads](#entradas)
7. [🔒 Autorização](#autorizacao)
8. [🔑 Segredos e configuração](#segredos)
9. [🐳 Containers](#containers)
10. [📜 Auditoria](#auditoria)
11. [📦 Dependências e CI](#dependencias)
12. [🧪 Como testar](#testar)
13. [⚠️ O que continua fora do escopo](#limites)

---

<a id="visao"></a>
## 1. 🗺️ Visão geral

| Ataque | Defesa | Onde |
|---|---|---|
| 🚪 Adivinhar a senha no login | bloqueio da conta após 8 erros em 15 min + limite por IP | `http/seguranca.go` |
| 🔑 Adivinhar a senha com uma **sessão roubada** | bloqueio após 8 senhas erradas nas operações que pedem senha | `protegeSenha` |
| 🌊 Enxurrada de requisições | limites por IP em toda a API, no cadastro e na validação | `limitador` |
| 🎭 CSRF (outro site agindo em seu nome) | conferência da **origem exata**, além do `SameSite=Strict` | `mesmaOrigem` |
| 🧬 XSS | React escapa tudo + **CSP** sem script inline | `cabecalhosSeguranca` |
| 🖼️ Clickjacking | `frame-ancestors 'none'` + `X-Frame-Options: DENY` | idem |
| 🍪 Roubo ou plantio de cookie | `HttpOnly`, `Secure`, `SameSite=Strict`, prefixo `__Host-` | `nomeCookie` |
| 🪪 Forjar o CN `NOME:CPF` | nome só aceita letras | `domain/usuario.go` |
| 📎 Upload malicioso | tamanho máximo, assinatura `%PDF-`, nome saneado, panic contido | `lerPDF`, `recuperar` |
| 🕵️ Ler dados de outro usuário | toda consulta filtra pelo usuário da sessão; PDFs cifrados por signatário | repositórios |
| 💉 Injeção de SQL | consultas 100% parametrizadas (pgx) | `postgres/` |
| 🗝️ Segredo padrão em produção | o servidor **se recusa a subir** | `cmd/server/main.go` |
| 🐳 Escapar do container | sem root, sem capacidades, FS somente leitura | `docker-compose.yml` |
| 📦 Biblioteca vulnerável | `govulncheck` + `npm audit` no CI; versão mínima do Go travada | `.github/workflows/ci.yml` |

---

<a id="forca-bruta"></a>
## 2. 🚪 Força bruta e abuso

> 🗣️ **Em palavras simples:** quem erra demais espera. Os contadores ficam na memória do servidor e cada resposta bloqueada diz em quanto tempo tentar de novo.

| Limite | Chave | Valor | Protege contra |
|---|---|---|---|
| 🔐 Senhas erradas no **login** | CPF | 8 em 15 min | adivinhar a senha de **uma conta**, de qualquer IP |
| 🚪 Tentativas de login | IP | 30 por minuto | testar uma senha comum em **muitas contas** |
| 🔑 Senha errada em assinar, baixar, exportar, revogar, renovar | usuário | 8 em 15 min | quem roubou a sessão e tenta descobrir a senha |
| 🪪 Cadastros | IP | 30 em 10 min | criar contas em massa (cada uma gera um par RSA) |
| 🔎 Cadastros **recusados** | IP | 20 em 10 min | sondar quais CPFs já têm conta |
| 🔍 Validações públicas | IP | 60 por minuto | usar a validação para gastar CPU |
| 🌐 Qualquer rota da API | IP | 600 por minuto | enxurrada genérica |

* ✅ Só as **falhas** contam para o bloqueio de senha: acertar zera o contador.
* 📬 Resposta bloqueada: **429** com o cabeçalho `Retry-After`.
* 🧠 O mapa de contadores tem teto de entradas, para o próprio limitador não virar um jeito de esgotar a memória.
* ⚖️ **Custo conhecido:** alguém que saiba o seu CPF pode travar o seu login por 15 minutos errando a senha de propósito. É o preço de impedir a força bruta; o bloqueio é curto e não apaga nada.

### 🌍 O IP verdadeiro

Em produção todo acesso chega pelo túnel da Cloudflare, então a conexão sempre parece vir do mesmo lugar. O IP real vem no cabeçalho `CF-Connecting-IP`, que a Cloudflare **sobrescreve** na borda. A aplicação só confia nele quando `IP_HEADER` está configurado (produção); localmente ele é **ignorado**, porque qualquer cliente poderia forjá-lo para burlar os limites.

Além disso, as filas de memória do [teste de carga](CARGA.md) (`LIMITE_ARGON2`, `LIMITE_PDF`) continuam valendo: mesmo dentro dos limites acima, o servidor nunca processa mais do que aguenta.

---

<a id="csrf"></a>
## 3. 🎭 CSRF e subdomínios irmãos

> 🗣️ **Em palavras simples:** CSRF é quando outro site faz o **seu** navegador mandar um pedido para cá, aproveitando que você está logado. O cookie `SameSite=Strict` barra sites de fora, mas tem um ponto cego: para o navegador, `studygo.d4ffy.com` e `lcr.d4ffy.com` são o **mesmo site**. Um app vizinho comprometido passaria.

Por isso toda requisição de escrita (`POST`) passa por `mesmaOrigem`, que exige a **origem exata**:

| Cabeçalho | Regra |
|---|---|
| `Sec-Fetch-Site` (navegadores atuais) | só `same-origin` ou `none` passam; `same-site` e `cross-site` → **403** |
| `Origin` (navegadores antigos) | o host precisa ser igual ao `Host` da requisição |
| nenhum dos dois (`curl`, scripts) | passa: sem navegador não há CSRF |

---

<a id="cabecalhos"></a>
## 4. 🧱 Cabeçalhos do navegador

| Cabeçalho | Valor | Para quê |
|---|---|---|
| `Content-Security-Policy` | `default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; ... object-src 'none'; frame-ancestors 'none'` | 🧬 mesmo que um XSS apareça, script inline ou de outro domínio **não executa** |
| `X-Frame-Options` | `DENY` | 🖼️ a página não abre dentro de `<iframe>` (clickjacking) |
| `X-Content-Type-Options` | `nosniff` | o navegador não "adivinha" tipos de arquivo |
| `Referrer-Policy` | `no-referrer` | links para fora não levam o endereço da página |
| `Permissions-Policy` | câmera, microfone, localização, pagamento e USB desligados | reduz o que um script poderia pedir |
| `Cross-Origin-Opener-Policy` | `same-origin` | isola a janela de páginas de outras origens |
| `Strict-Transport-Security` | `max-age=31536000` (só em produção) | 🔐 o navegador nunca mais tenta HTTP |
| `Cache-Control` | `no-store` em toda a API | PDFs e dados pessoais não ficam em cache |
| `Cross-Origin-Resource-Policy` | `same-origin` na API (menos `/api/ac/`) | outros sites não embutem respostas da API |

📌 A CSP só permite `'unsafe-inline'` para **estilos** (o React usa o atributo `style`). Para **scripts** não há exceção. O frontend não usa `innerHTML`, `eval` nem guarda nada em `localStorage`.

📌 Os arquivos públicos da AC (`/api/ac/raiz.crt`, `lcr.crl`...) ficam de fora do CORP de propósito: qualquer validador precisa conseguir buscá-los.

---

<a id="sessao"></a>
## 5. 🍪 Sessão e cookie

| Proteção | Detalhe |
|---|---|
| 🎟️ Token | 256 bits aleatórios; o banco guarda só o **SHA-256** |
| 🙈 `HttpOnly` | o JavaScript não lê o cookie |
| 🔐 `Secure` | só trafega em HTTPS (produção) |
| 🚧 `SameSite=Strict` | não acompanha requisições vindas de outros sites |
| 🏷️ Prefixo `__Host-` (produção) | o navegador só aceita esse nome com `Secure`, `Path=/` e **sem** `Domain`. Outro app em `*.d4ffy.com` **não consegue plantar** um cookie de sessão para cá (fixação de sessão) |
| ⏱️ Validade | 8 horas; sessões vencidas são apagadas a cada novo login |
| 🕵️ Login | mesma resposta e mesmo tempo para CPF inexistente e senha errada |

---

<a id="entradas"></a>
## 6. 🧪 Validação de entradas e uploads

| Entrada | Regra | Por quê |
|---|---|---|
| 👤 **Nome** | 3 a 80 caracteres, **só letras**, espaço, `'`, `-`, `.` | o certificado usa o CN `NOME:CPF`. Um nome como `MARIA:11144477735` forjaria um CN com o CPF de outra pessoa |
| 🔢 **CPF** | 11 dígitos com verificadores válidos | |
| 🔑 **Senha** | 8 a 128 caracteres | o teto evita senhas gigantes só para gastar CPU |
| 📝 **Motivo** | até 120 caracteres, sem caracteres de controle | vai para o carimbo e para dentro da assinatura |
| 📄 **Nome do arquivo** | sem diretórios, aspas, controle nem caracteres invisíveis de direção; máx. 100 | impede `../../etc/passwd`, injeção de cabeçalho e nomes disfarçados (`fatura‮fdp.exe`) |
| 📎 **PDF** | até 20 MiB e precisa começar com `%PDF-` | |
| 📨 **JSON** | corpo limitado a 64 KiB | |
| 📬 **Cabeçalhos HTTP** | até 64 KiB, com timeouts de leitura | conexões lentas (slowloris) |

💥 Um PDF malformado que faça uma biblioteca entrar em pânico vira **500 genérico** (`recuperar`): o servidor não cai e o detalhe vai só para o log.

---

<a id="autorizacao"></a>
## 7. 🔒 Autorização

* 🧍 O usuário vem **sempre** da sessão, nunca de um parâmetro: não existe "id do usuário" na URL.
* 📂 Documentos: a consulta exige uma chave de acesso para um certificado **do próprio usuário**. Documento alheio responde **404**, sem revelar que existe.
* 🔐 Mesmo com acesso ao banco, o PDF está [cifrado por signatário](CERTIFICADOS.md#armazenamento).
* 🔁 Operações sensíveis (assinar, baixar, exportar, revogar, renovar) pedem a **senha de novo**: uma sessão roubada sozinha não basta.

---

<a id="segredos"></a>
## 8. 🔑 Segredos e configuração

* 📄 O `.env` nunca vai para o Git (`.gitignore` e `.dockerignore`).
* 🧪 As senhas do `.env.example` são **públicas**, só para rodar localmente.
* 🛑 Em produção (`COOKIE_SECURE=true`) o servidor **não sobe** se:
  * `AC_SENHA` ou a senha do banco forem as de desenvolvimento;
  * `RATE_LIMIT=off` (os limites desligados);
  * `PUBLIC_URL` não for `https://`.
* 🎲 O script de deploy gera `AC_SENHA` e `POSTGRES_PASSWORD` aleatórias, com permissão `600`.

---

<a id="containers"></a>
## 9. 🐳 Containers

| Medida | App | Banco |
|---|:---:|:---:|
| 🙅 Roda sem root | ✅ (uid 10001) | ✅ (usuário `postgres`) |
| 🧱 Sistema de arquivos somente leitura | ✅ (só `/tmp` em memória) | — |
| ✂️ Capacidades do kernel | **nenhuma** | só as 5 que o Postgres precisa |
| 🚫 `no-new-privileges` | ✅ | ✅ |
| 🧠 Teto de memória (produção) | 1,5 GiB | 768 MiB |
| 🌐 Porta exposta | só `127.0.0.1` em produção | **nenhuma** |

Em produção o Docker é **rootless** (usuário `lcr`): mesmo o "root" de dentro de um container é um usuário comum na máquina.

---

<a id="auditoria"></a>
## 10. 📜 Auditoria

Eventos de segurança saem no log como `auditoria`, com IP, rota e usuário:

```
level=INFO msg=auditoria evento=login_falhou ip=203.0.113.7 rota="POST /api/login" cpf=***.982.247-**
level=INFO msg=auditoria evento=taxa_excedida ip=203.0.113.7 rota="POST /api/login" limite=login_cpf
level=INFO msg=auditoria evento=assinatura ip=198.51.100.4 rota="POST /api/assinar" usuario=12 resultado=ok
level=INFO msg=auditoria evento=origem_rejeitada ip=... origin=https://malicioso.example
```

| Evento | Quando |
|---|---|
| `cadastro`, `cadastro_recusado` | conta criada ou recusada |
| `login`, `login_falhou` | entrada com sucesso ou senha errada |
| `assinatura`, `download_documento`, `exportacao_pfx`, `revogacao`, `renovacao` | operação sensível, com `resultado=ok`, `senha_incorreta` ou `recusado` |
| `taxa_excedida` | algum limite barrou a requisição |
| `origem_rejeitada` | tentativa de CSRF |

🙈 O log **nunca** recebe senha, token de sessão nem CPF completo (só `***.982.247-**`).

```bash
docker compose logs app | grep auditoria          # local
docker compose -p lcr logs app | grep auditoria   # produção (como o usuário lcr)
```

---

<a id="dependencias"></a>
## 11. 📦 Dependências e CI

* 🔎 [`.github/workflows/ci.yml`](../.github/workflows/ci.yml) roda a cada push: `gofmt`, `go vet`, testes com detector de corrida, **`govulncheck`**, build do frontend, **`npm audit`** e build da imagem.
* 📌 O `go.mod` exige **Go ≥ 1.27.4**. As versões anteriores têm 12 falhas conhecidas no servidor HTTP (entre elas a `GO-2026-6859`, um crash envolvendo o `MaxBytesReader` usado no upload). Compilar com Go antigo **falha**, em vez de gerar um binário vulnerável.
* ♻️ O deploy usa `build --pull`: sempre parte das imagens base atuais.

```bash
cd backend  && go run golang.org/x/vuln/cmd/govulncheck@latest ./...
cd frontend && npm audit
```

---

<a id="testar"></a>
## 12. 🧪 Como testar

[`scripts/e2e-seguranca.sh`](../scripts/e2e-seguranca.sh) **ataca a instalação local de verdade** e confere 54 itens: cabeçalhos, cookie, CSRF (inclusive subdomínio irmão), entradas maliciosas, uploads, acesso entre usuários, travessia de diretório, endurecimento do container, os três bloqueios de força bruta e a trilha de auditoria.

```bash
docker compose up -d --build
bash scripts/e2e-seguranca.sh
```

⚠️ Ele estoura os limites de propósito. Depois, rode `docker compose restart app` para zerar os contadores.

Testes unitários das mesmas defesas: `backend/internal/adapter/primary/http/seguranca_test.go` e `backend/internal/domain/domain_test.go`.

---

<a id="limites"></a>
## 13. ⚠️ O que continua fora do escopo

* 🪪 **Identidade:** qualquer pessoa cria conta com um CPF válido. Não há verificação de que o CPF é dela.
* 🧠 **Limites em memória:** os contadores zeram quando o app reinicia e não são compartilhados entre várias instâncias. Para mais de uma instância, precisariam ir para o Redis ou para o banco.
* 🔁 **Sem segundo fator (2FA)** e sem troca ou recuperação de senha: a senha cifra a chave privada, então quem a esquece perde o acesso à chave e aos documentos, e nem consegue revogar o certificado. Um fluxo de recuperação exigiria reemitir o certificado com validação de identidade.
* 🔧 **Sem HSM:** as chaves ficam no banco, cifradas com a senha.
* 📣 **Nomes na lista de revogados:** a página pública da AC mostra o nome do titular de cada certificado revogado, para fins didáticos.
