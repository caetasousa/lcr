# 🎬 Roteiro de demonstração (prints para o relatório)

Suba o projeto com `docker compose up -d --build` e abra http://localhost:8080.

| # | Passo | O que mostrar |
|:---:|---|---|
| 1️⃣ | 🪪 **Criar conta** com um CPF de teste (`529.982.247-25` ou `111.444.777-35`) | Na aba *Meu certificado*: subject `NOME:CPF`, issuer, serial, usos e o PEM |
| 2️⃣ | 🖋️ **Assinar**: envie um PDF, clique na página para posicionar o carimbo, arraste ou redimensione, informe a senha | O PDF baixado com o carimbo visível |
| 3️⃣ | 🔍 **Validar** o PDF assinado | ✅ três verificações verdes e a cadeia de certificados |
| 4️⃣ | 🔧 **Alterar o PDF**: troque um único byte do conteúdo (editor hexadecimal ou o Bloco 8 do laboratório) e valide de novo | ❌ *Documento alterado após a assinatura* (`Message digest mismatch`) |
| 5️⃣ | 👥 **Dois signatários**: entre com outra conta e assine o PDF já assinado | A validação mostra as duas assinaturas |
| 6️⃣ | 🚫 **Revogar** o certificado em *Meu certificado* | O PDF antigo continua válido, com ⚠️ aviso de revogação posterior. Assinar de novo é bloqueado. |
| 7️⃣ | 📜 Baixar a **LCR** em *AC ACME* | O serial revogado aparece na lista (comando abaixo) |
| 8️⃣ | 💾 **Exportar `.pfx`** e instalar no Windows (item 4.3 do laboratório) | Instale também a AC Raiz G2 em "Autoridades de Certificação Raiz Confiáveis" |

## 🧪 Conferência pela linha de comando

```bash
# 🏛️ baixar a cadeia
curl -s localhost:8080/api/ac/raiz.crt          -o raiz.crt
curl -s localhost:8080/api/ac/intermediaria.crt -o inter.crt

# 🔍 inspecionar os certificados
openssl x509 -in raiz.crt  -noout -text
openssl x509 -in inter.crt -noout -text

# 🔗 verificar a cadeia do certificado do usuário (baixado em "Meu certificado")
openssl verify -CAfile raiz.crt -untrusted inter.crt meu_certificado.crt

# 📜 ler a LCR
curl -s localhost:8080/api/ac/lcr.crl -o ac_acme.crl
openssl crl -inform DER -in ac_acme.crl -noout -text

# 💾 inspecionar o .pfx
openssl pkcs12 -in meu_certificado.pfx -info -noout
```

### ✂️ Extrair o CMS de dentro do PDF

```bash
# pega o conteúdo hexadecimal de /Contents (primeira assinatura) e converte para DER
python3 - contrato_assinado.pdf <<'EOF'
import re, sys
dados = open(sys.argv[1], 'rb').read()
hexa = re.search(rb'/Contents\s*<([0-9A-Fa-f]+)>', dados).group(1)
open('assinatura.p7s', 'wb').write(bytes.fromhex(hexa.decode()).rstrip(b'\0'))
EOF
openssl cms -cmsout -inform DER -in assinatura.p7s -print -noout | head -60
```

Aparecem o `messageDigest` (SHA-256), o `SigningCertificateV2` e os três certificados da cadeia.
