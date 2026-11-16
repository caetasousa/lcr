#!/usr/bin/env bash
# 🛡️ Teste de ponta a ponta das defesas de segurança.
# Ataca a instalação local de verdade (API + containers) e confere cada defesa.
#
#   docker compose up -d --build
#   bash scripts/e2e-seguranca.sh
#
# ⚠️ Roda só contra a instalação LOCAL: ele estoura de propósito os limites de
# tentativas (depois dele, espere 1 minuto para o limite de login por IP
# reabrir, ou reinicie o app com: docker compose restart app).
set -u
B=${B:-http://localhost:8080}
TRABALHO=$(mktemp -d); cd "$TRABALHO"
ok=0; falha=0
check() { if [ "$1" = "$2" ]; then echo "  ✅ $3"; ok=$((ok+1)); else echo "  ❌ $3 (esperado: $2, obtido: $1)"; falha=$((falha+1)); fi; }
contem() { case "$1" in *"$2"*) echo "  ✅ $3"; ok=$((ok+1));; *) echo "  ❌ $3 (não contém: $2 | obtido: $1)"; falha=$((falha+1));; esac; }
cpf() { python3 -c "
import random
n=[random.randint(0,9) for _ in range(9)]
for k in (10,11):
    s=sum(d*(k-i) for i,d in enumerate(n)); r=(s*10)%11; n.append(0 if r==10 else r)
print(''.join(map(str,n)))"; }
J='Content-Type: application/json'
st() { curl -s -o /dev/null -w '%{http_code}' "$@"; }
cab() { curl -s -D - -o /dev/null "$@" | tr -d '\r'; }

python3 - <<'PY'
objs=[b"<< /Type /Catalog /Pages 2 0 R >>", b"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
b"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>"]
st=b"BT /F1 24 Tf 72 760 Td (Contrato) Tj ET"
objs.append(b"<< /Length %d >>\nstream\n"%len(st)+st+b"\nendstream")
objs.append(b"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
out=b"%PDF-1.7\n"; offs=[]
for i,o in enumerate(objs,1):
    offs.append(len(out)); out+=b"%d 0 obj\n"%i+o+b"\nendobj\n"
x=len(out); out+=b"xref\n0 %d\n0000000000 65535 f \n"%(len(objs)+1)
for o in offs: out+=b"%010d 00000 n \n"%o
out+=b"trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n"%(len(objs)+1,x)
open("ok.pdf","wb").write(out)
open("falso.pdf","wb").write(b"MZ isto e um executavel renomeado")
open("lixo.pdf","wb").write(b"%PDF-1.7\n"+bytes(range(256))*200+b"\n%%EOF")
PY
head -c $((22*1024*1024)) /dev/zero > grande.pdf

CPF_A=$(cpf); CPF_B=$(cpf)
curl -s -o /dev/null -D cad.txt -c a.txt $B/api/cadastro -H "$J" -d "{\"nome\":\"Alice Segura\",\"cpf\":\"$CPF_A\",\"senha\":\"senha-da-alice-1\"}"
curl -s -o /dev/null -c b.txt $B/api/cadastro -H "$J" -d "{\"nome\":\"Bruno Seguro\",\"cpf\":\"$CPF_B\",\"senha\":\"senha-do-bruno-1\"}"

echo "🧱 1. Cabeçalhos de segurança"
H=$(cab $B/); A=$(cab -b a.txt $B/api/eu)
contem "$H" "Content-Security-Policy: default-src 'self'" "CSP na página"
contem "$H" "frame-ancestors 'none'" "CSP proíbe embutir em iframe (clickjacking)"
contem "$H" "X-Frame-Options: DENY" "X-Frame-Options"
contem "$H" "X-Content-Type-Options: nosniff" "nosniff"
contem "$H" "Referrer-Policy: no-referrer" "Referrer-Policy"
contem "$H" "Permissions-Policy:" "Permissions-Policy"
contem "$A" "Cache-Control: no-store" "API nunca é guardada em cache"
check "$(echo "$H" | grep -ci 'access-control-allow-origin')" "0" "sem CORS aberto"
check "$(echo "$H" | grep -ci '^server:\|x-powered-by')" "0" "não anuncia a tecnologia do servidor"

echo "🍪 2. Cookie de sessão"
C=$(grep -i '^set-cookie' cad.txt)
contem "$C" "HttpOnly" "HttpOnly (JavaScript não lê)"
contem "$C" "SameSite=Strict" "SameSite=Strict"
check "$(st -b 'sessao=0000000000000000000000000000000000000000000000000000000000000000' $B/api/eu)" "401" "cookie inventado → 401"
check "$(st $B/api/eu)" "401" "sem cookie → 401"

echo "🎭 3. CSRF: requisições de outro site são recusadas"
check "$(st -b a.txt $B/api/certificado/revogar -H "$J" -H 'Origin: https://malicioso.example' -H 'Sec-Fetch-Site: cross-site' -d '{"senha":"x","motivo":1}')" "403" "outro site → 403"
check "$(st -b a.txt $B/api/certificado/revogar -H "$J" -H 'Origin: https://studygo.d4ffy.com' -H 'Sec-Fetch-Site: same-site' -d '{"senha":"x","motivo":1}')" "403" "subdomínio irmão → 403"
check "$(st -b a.txt $B/api/certificado/revogar -H "$J" -H 'Origin: http://outro-host:8080' -d '{"senha":"x","motivo":1}')" "403" "Origin de outro host (navegador antigo) → 403"
check "$(st -b a.txt $B/api/logout -H 'Sec-Fetch-Site: same-origin' -X POST)" "200" "mesma origem passa"
curl -s -o /dev/null -c a.txt $B/api/login -H "$J" -d "{\"cpf\":\"$CPF_A\",\"senha\":\"senha-da-alice-1\"}"

echo "🧪 4. Validação de entradas"
check "$(st $B/api/cadastro -H "$J" -d "{\"nome\":\"Maria Silva:11144477735\",\"cpf\":\"$(cpf)\",\"senha\":\"senha-forte-123\"}")" "400" "nome com ':' (forjaria o CN NOME:CPF) → 400"
check "$(st $B/api/cadastro -H "$J" -d "{\"nome\":\"<script>alert(1)</script>\",\"cpf\":\"$(cpf)\",\"senha\":\"senha-forte-123\"}")" "400" "nome com HTML → 400"
check "$(st $B/api/cadastro -H "$J" -d "{\"nome\":\"Fulano Teste\",\"cpf\":\"$(cpf)\",\"senha\":\"$(head -c 300 /dev/zero | tr '\0' a)\"}")" "400" "senha de 300 caracteres → 400"
check "$(st $B/api/cadastro -H "$J" -d "{\"nome\":\"Fulano Teste\",\"cpf\":\"12345678900\",\"senha\":\"senha-forte-123\"}")" "400" "CPF com dígito verificador errado → 400"
check "$(st $B/api/login -H "$J" -d "{\"cpf\":\"' OR 1=1 --\",\"senha\":\"x\"}")" "401" "injeção de SQL no login → 401 (consulta parametrizada)"
check "$(st $B/api/login -H "$J" --data-binary @grande.pdf)" "400" "corpo JSON gigante → 400"

echo "📎 5. Upload de arquivos"
check "$(st $B/api/validar -F arquivo=@falso.pdf)" "422" "arquivo que não é PDF → 422"
check "$(st $B/api/validar -F arquivo=@grande.pdf)" "413" "arquivo de 22 MB → 413"
L=$(st $B/api/validar -F arquivo=@lixo.pdf)
check "$( [ "$L" != "500" ] && [ "$L" != "000" ] && echo tratado)" "tratado" "PDF corrompido é tratado sem erro interno (status $L)"
check "$(st $B/api/ac)" "200" "servidor continua de pé depois do PDF corrompido"
curl -s -D sig.txt -o s.pdf -b a.txt $B/api/assinar -F 'arquivo=@ok.pdf;filename=../../etc/passwd"; rm -rf.pdf' -F pagina=1 -F x1=40 -F y1=40 -F x2=290 -F y2=106 -F senha=senha-da-alice-1 -F "motivo=$(printf 'Aceite\r\nX-Injetado: 1')"
D=$(grep -i '^content-disposition' sig.txt | tr -d '\r')
check "$(echo "$D" | grep -c '\.\./\|/etc/')" "0" "nome de arquivo com ../ é saneado ($D)"
check "$(grep -ci '^x-injetado' sig.txt)" "0" "quebra de linha no motivo não injeta cabeçalho"
DOC=$(grep -i '^x-documento-id' sig.txt | tr -dc 0-9)

echo "🔒 6. Autorização: um usuário não alcança o que é de outro"
check "$(st -b b.txt $B/api/documentos/$DOC -H "$J" -d '{"senha":"senha-do-bruno-1"}')" "404" "Bruno não baixa o documento da Alice → 404"
check "$(curl -s -b b.txt $B/api/documentos | python3 -c 'import json,sys;print(len(json.load(sys.stdin)))')" "0" "Bruno não vê o documento da Alice na lista"
check "$(st $B/api/documentos)" "401" "lista de documentos sem login → 401"
check "$(st $B/api/certificado.crt)" "401" "certificado sem login → 401"

echo "📁 7. Arquivos estáticos"
T=$(curl -s --path-as-is "$B/../../../../etc/passwd"; curl -s --path-as-is "$B/assets/..%2f..%2f..%2fetc%2fpasswd")
check "$(echo "$T" | grep -c 'root:')" "0" "travessia de diretório não lê /etc/passwd"

echo "🐳 8. Container endurecido"
check "$(docker exec lcr-app-1 id -u)" "10001" "app roda sem root (uid 10001)"
check "$(docker exec lcr-app-1 sh -c 'touch /app/x 2>&1 >/dev/null; echo $?' | tail -1)" "1" "sistema de arquivos somente leitura"
check "$(docker exec lcr-app-1 sh -c "grep CapEff /proc/1/status | awk '{print \$2}'")" "0000000000000000" "nenhuma capacidade do kernel"
check "$(docker exec lcr-app-1 sh -c "grep NoNewPrivs /proc/1/status | awk '{print \$2}'")" "1" "não pode ganhar privilégios"
check "$(docker port lcr-db-1 2>/dev/null | wc -l)" "0" "banco de dados não é exposto fora do Docker"

echo "🔑 9. Força bruta na senha das operações (sessão roubada)"
for i in $(seq 8); do S=$(st -b a.txt $B/api/certificado/pfx -H "$J" -d '{"senha":"chute-errado"}'); done
check "$S" "403" "8 senhas erradas → 403 em cada uma"
R=$(cab -b a.txt $B/api/certificado/pfx -H "$J" -d '{"senha":"chute-errado"}')
contem "$R" " 429" "a 9ª é bloqueada → 429"
contem "$R" "Retry-After:" "resposta informa quando tentar de novo"
check "$(st -b a.txt $B/api/certificado/pfx -H "$J" -d '{"senha":"senha-da-alice-1"}')" "429" "durante o bloqueio nem a senha certa passa"
check "$(st -b b.txt $B/api/certificado/pfx -H "$J" -d '{"senha":"senha-do-bruno-1"}')" "200" "o bloqueio é só da Alice: Bruno segue normal"

echo "🚪 10. Força bruta no login de uma conta"
for i in $(seq 8); do S=$(st $B/api/login -H "$J" -d "{\"cpf\":\"$CPF_B\",\"senha\":\"chute-$i\"}"); done
check "$S" "401" "8 senhas erradas → 401 em cada uma"
check "$(st $B/api/login -H "$J" -d "{\"cpf\":\"$CPF_B\",\"senha\":\"chute-9\"}")" "429" "a 9ª tentativa na mesma conta → 429"
check "$(st $B/api/login -H "$J" -d "{\"cpf\":\"$CPF_B\",\"senha\":\"senha-do-bruno-1\"}")" "429" "conta bloqueada temporariamente, mesmo com a senha certa"
check "$(st $B/api/login -H "$J" -d "{\"cpf\":\"$CPF_A\",\"senha\":\"senha-da-alice-1\"}")" "200" "outras contas não são afetadas"

echo "🌊 11. Enxurrada de tentativas a partir de um endereço"
N429=0
for i in $(seq 45); do
  [ "$(st $B/api/login -H "$J" -H "CF-Connecting-IP: 198.51.100.$i" -H "X-Forwarded-For: 198.51.100.$i" -d "{\"cpf\":\"$(printf '%011d' $i)\",\"senha\":\"x\"}")" = "429" ] && N429=$((N429+1))
done
check "$( [ $N429 -gt 0 ] && echo bloqueou)" "bloqueou" "45 logins seguidos do mesmo IP são barrados ($N429 com 429)"
echo "     ↳ forjar CF-Connecting-IP / X-Forwarded-For não burla o limite fora do proxy confiável"

echo "📜 12. Trilha de auditoria"
LOG=$(docker logs --since 5m lcr-app-1 2>&1 | grep auditoria)
contem "$LOG" "evento=login_falhou" "login com senha errada foi registrado"
contem "$LOG" "evento=origem_rejeitada" "tentativa de CSRF foi registrada"
contem "$LOG" "evento=taxa_excedida" "bloqueio por excesso de tentativas foi registrado"
contem "$LOG" "evento=assinatura" "assinatura foi registrada"
check "$(echo "$LOG" | grep -c "$CPF_B\|senha-do-bruno\|chute-")" "0" "o log não contém CPF completo nem senhas"

echo; echo "Resultado: $ok passaram, $falha falharam"; [ $falha -eq 0 ]
