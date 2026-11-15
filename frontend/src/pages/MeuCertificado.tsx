import { useCallback, useState } from 'react'
import { api, baixar, dataHora, type Eu } from '../api'
import Modal from '../components/Modal'
import Icone from '../components/Icone'

type Acao = 'pfx' | 'revogar' | 'renovar' | null

const MOTIVOS: [number, string][] = [
  [1, 'Chave comprometida (perda, roubo, vazamento)'],
  [3, 'Mudança de vínculo com a ACME'],
  [4, 'Substituído por outro certificado'],
  [5, 'Cessação de operação'],
  [0, 'Não especificado'],
]

const TITULOS: Record<Exclude<Acao, null>, string> = {
  pfx: 'Exportar certificado A1 (.pfx)',
  revogar: 'Revogar certificado',
  renovar: 'Emitir novo certificado',
}

export default function MeuCertificado({ eu, onMudou }: { eu: Eu; onMudou: () => Promise<void> }) {
  const c = eu.certificado
  const valido = c.situacao === 'válido'
  const [acao, setAcao] = useState<Acao>(null)
  const [senha, setSenha] = useState('')
  const [motivo, setMotivo] = useState(1)
  const [erro, setErro] = useState('')
  const [ocupado, setOcupado] = useState(false)

  const abrir = (a: Acao) => {
    setAcao(a)
    setSenha('')
    setErro('')
  }
  const fechar = useCallback(() => !ocupado && setAcao(null), [ocupado])

  const confirmar = async (e: React.FormEvent) => {
    e.preventDefault()
    setOcupado(true)
    setErro('')
    try {
      if (acao === 'pfx') baixar(await api.exportarPfx(senha), 'meu_certificado.pfx')
      if (acao === 'revogar') await api.revogar(senha, motivo)
      if (acao === 'renovar') await api.renovar(senha)
      setAcao(null)
      if (acao !== 'pfx') await onMudou()
    } catch (err) {
      setErro((err as Error).message)
    } finally {
      setOcupado(false)
    }
  }

  return (
    <div>
      <header className="cabecalho-pagina revelar">
        <span className="rotulo">Identidade digital</span>
        <h1 className="titulo">Meu certificado</h1>
      </header>

      <div className="painel revelar" style={{ '--i': 1 } as React.CSSProperties}>
        <div className="cert-topo">
          <div className="cert-titular">
            <div className="cert-icone"><Icone nome="certificado" tamanho={24} /></div>
            <div>
              <h2 className="subtitulo">{eu.nome}</h2>
              <span className="sutil">CPF {eu.cpf} · tipo A1, emitido pela AC ACME</span>
            </div>
          </div>
          <span className={'chip ' + (valido ? 'ok' : 'falha')}><span className="ponto" />{c.situacao.toUpperCase()}</span>
        </div>

        {c.situacao === 'revogado' && (
          <div className="nota perigo">
            <Icone nome="alerta" />
            <span>
              Revogado em {dataHora(c.revogado_em)}, motivo: {c.motivo_revogacao}. O número de série já consta na LCR.
              Documentos assinados antes disso continuam válidos.
            </span>
          </div>
        )}

        <dl className="detalhes">
          <dt>Titular (subject)</dt><dd><code>{c.titular}</code></dd>
          <dt>Emissor (issuer)</dt><dd><code>{c.emissor}</code></dd>
          <dt>Número de série</dt><dd><code>{c.serial}</code></dd>
          <dt>Validade</dt><dd>{dataHora(c.valido_de)} → {dataHora(c.valido_ate)}</dd>
          <dt>Algoritmos</dt><dd>{c.algoritmo} · {c.tamanho_chave} bits</dd>
          <dt>Usos permitidos</dt>
          <dd className="chips">{c.usos_chave.map((u) => <span key={u} className="chip lime">{u}</span>)}</dd>
          <dt>Distribuição da LCR</dt><dd><code>{c.lcr.join(', ')}</code></dd>
          <dt>Impressão digital SHA-256</dt><dd><code>{c.fingerprint_sha256}</code></dd>
        </dl>

        <details>
          <summary>Certificado em PEM</summary>
          <pre className="pem">{c.pem}</pre>
        </details>

        <div className="acoes" style={{ marginTop: 'var(--e4)' }}>
          <a className="botao" href="/api/certificado.crt"><Icone nome="baixar" tamanho={14} /> Baixar .crt</a>
          {valido && <button onClick={() => abrir('pfx')}><Icone nome="chave" tamanho={14} /> Exportar .pfx (A1)</button>}
          {valido && <button className="perigo" onClick={() => abrir('revogar')}><Icone nome="x" tamanho={14} /> Revogar</button>}
          {!valido && <button className="primario" onClick={() => abrir('renovar')}><Icone nome="certificado" tamanho={14} /> Emitir novo certificado</button>}
        </div>
      </div>

      <div className="painel revelar" style={{ '--i': 2 } as React.CSSProperties}>
        <span className="rotulo">Proteção da chave privada</span>
        <pre className="diagrama mono" style={{ marginTop: 'var(--e2)' }}>
          senha <span className="fraco">──</span> <span className="destaque">Argon2id</span> (salt 16 B, t=3, m=64 MiB) <span className="fraco">──▶</span> chave AES-256{'\n'}
          chave RSA (PKCS#8) <span className="fraco">──</span> <span className="destaque">AES-256-GCM</span> (nonce 12 B) <span className="fraco">──▶</span> blob no PostgreSQL
        </pre>
        <p className="sutil" style={{ margin: 0 }}>
          O servidor guarda só o blob cifrado e um hash Argon2id da senha. Para assinar, exportar ou revogar, a senha é pedida
          de novo. Com a senha errada, a tag de autenticação do GCM não confere e a chave não é liberada.
        </p>
      </div>

      {acao && (
        <Modal titulo={TITULOS[acao]} onFechar={fechar}>
          <form onSubmit={confirmar}>
            {acao === 'pfx' && (
              <p className="sutil">
                Gera um PKCS#12 com o certificado, a chave privada e a cadeia da AC, protegido pela sua senha (AES-256 +
                PBKDF2). Dá para instalar no Windows como no item 4.3 do laboratório.
              </p>
            )}
            {acao === 'revogar' && (
              <>
                <div className="nota perigo">
                  <Icone nome="alerta" />
                  <span>A série <b>{c.serial}</b> entra na LCR e você não poderá mais assinar com este certificado. Não dá para desfazer.</span>
                </div>
                <label>
                  <span>Motivo</span>
                  <select value={motivo} onChange={(e) => setMotivo(Number(e.target.value))}>
                    {MOTIVOS.map(([v, t]) => <option key={v} value={v}>{t}</option>)}
                  </select>
                </label>
              </>
            )}
            {acao === 'renovar' && <p className="sutil">Um novo par de chaves RSA-3072 será gerado e a AC emitirá um novo certificado.</p>}
            <label>
              <span>Sua senha</span>
              <input type="password" value={senha} onChange={(e) => setSenha(e.target.value)} autoFocus required />
            </label>
            {erro && <p className="erro" role="alert">{erro}</p>}
            <div className="acoes direita" style={{ marginTop: 'var(--e3)' }}>
              <button type="button" onClick={() => setAcao(null)} disabled={ocupado}>Cancelar</button>
              <button className={acao === 'revogar' ? 'perigo cheio' : 'primario'} disabled={ocupado}>
                {ocupado ? <><span className="giro" /> Aguarde…</> : acao === 'pfx' ? 'Exportar' : acao === 'revogar' ? 'Revogar' : 'Emitir'}
              </button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}
