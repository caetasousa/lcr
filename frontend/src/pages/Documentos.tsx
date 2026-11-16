import { useEffect, useState } from 'react'
import { api, baixar, dataHora, type Documento } from '../api'
import Icone from '../components/Icone'
import Modal from '../components/Modal'
import { navegar } from '../App'

export default function Documentos({ onValidar }: { onValidar: (b: Blob, nome: string) => void }) {
  const [docs, setDocs] = useState<Documento[] | null>(null)
  const [erro, setErro] = useState('')

  useEffect(() => {
    api.documentos().then(setDocs).catch((e) => setErro(e.message))
  }, [])

  // Os PDFs ficam cifrados no servidor. A senha é pedida uma vez e guardada
  // só na memória desta página, para abrir outros documentos sem redigitar.
  const [senha, setSenha] = useState('')
  const [pedido, setPedido] = useState<{ d: Documento; validar: boolean } | null>(null)
  const [digitada, setDigitada] = useState('')
  const [erroSenha, setErroSenha] = useState('')
  const [ocupado, setOcupado] = useState(false)

  const abrir = async (d: Documento, validar: boolean, s: string) => {
    const b = await api.documento(d.id, s)
    setSenha(s)
    if (validar) onValidar(b, d.nome)
    else baixar(b, d.nome)
  }

  const acao = async (d: Documento, validar: boolean) => {
    setErro('')
    if (!senha) {
      setPedido({ d, validar })
      return
    }
    try {
      await abrir(d, validar, senha)
    } catch (e) {
      setSenha('')
      setErro((e as Error).message)
    }
  }

  const confirmar = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!pedido) return
    setOcupado(true)
    setErroSenha('')
    try {
      await abrir(pedido.d, pedido.validar, digitada)
      setPedido(null)
      setDigitada('')
    } catch (er) {
      setErroSenha((er as Error).message)
    } finally {
      setOcupado(false)
    }
  }

  return (
    <div>
      <header className="cabecalho-pagina revelar">
        <span className="rotulo">Histórico</span>
        <h1 className="titulo">Meus documentos</h1>
      </header>
      {erro && <p className="erro">{erro}</p>}
      {!docs && !erro && <div className="carregando"><span className="giro" /> Carregando…</div>}

      {docs && docs.length === 0 && (
        <div className="painel vazio revelar">
          <div className="icone-grande"><Icone nome="documento" tamanho={24} /></div>
          <p className="subtitulo">Nenhum documento assinado ainda</p>
          <p className="sutil">Os PDFs que você assinar ficam guardados aqui, cifrados.</p>
          <button className="primario grande" onClick={() => navegar('/assinar')}><Icone nome="caneta" tamanho={16} /> Assinar um documento</button>
        </div>
      )}

      {docs && docs.length > 0 && (
        <div className="painel revelar" style={{ '--i': 1 } as React.CSSProperties}>
          <div className="painel-cabecalho">
            <span className="sutil">
              {docs.length} documento(s) · <Icone nome="chave" tamanho={12} /> guardados cifrados: só quem assinou consegue abrir
            </span>
            <button className="primario" onClick={() => navegar('/assinar')}><Icone nome="caneta" tamanho={14} /> Novo</button>
          </div>
          <div className="tabela-wrap">
            <table>
              <thead>
                <tr>
                  <th>Documento</th>
                  <th>Assinado em</th>
                  <th>Assinado por</th>
                  <th>Pág.</th>
                  <th>SHA-256</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {docs.map((d) => (
                  <tr key={d.id}>
                    <td>
                      <div className="doc-cabecalho">
                        <div className="doc-icone"><Icone nome="documento" tamanho={16} /></div>
                        <div>
                          <div className="nome">{d.nome}</div>
                          <div className="fraco" style={{ fontSize: 12 }}>{(d.tamanho / 1024).toFixed(0)} KB</div>
                        </div>
                      </div>
                    </td>
                    <td className="sutil">{dataHora(d.assinado_em)}</td>
                    <td className="sutil" style={{ fontSize: 13 }}>
                      {d.signatarios.map((n) => <div key={n}>{n}</div>)}
                    </td>
                    <td className="sutil">{d.pagina}</td>
                    <td><code title={d.sha256_assinado}>{d.sha256_assinado.slice(0, 12)}…</code></td>
                    <td className="acoes-linha">
                      <div className="acoes direita">
                        <button onClick={() => acao(d, true)}><Icone nome="escudo" tamanho={14} /> Validar</button>
                        <button onClick={() => acao(d, false)}><Icone nome="baixar" tamanho={14} /> Baixar</button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
      {pedido && (
        <Modal titulo="Abrir documento cifrado" onFechar={() => setPedido(null)}>
          <form onSubmit={confirmar}>
            <p className="sutil">
              Os documentos ficam guardados cifrados (AES-256-GCM). A sua senha libera a sua chave privada, que abre a chave
              deste documento. Antes da entrega, o SHA-256 é conferido com o registrado na assinatura.
            </p>
            <label>
              <span>Sua senha</span>
              <input type="password" value={digitada} onChange={(e) => setDigitada(e.target.value)} autoFocus required />
            </label>
            {erroSenha && <p className="erro" role="alert">{erroSenha}</p>}
            <div className="acoes direita" style={{ marginTop: 'var(--e3)' }}>
              <button type="button" onClick={() => setPedido(null)} disabled={ocupado}>Cancelar</button>
              <button className="primario" disabled={ocupado}>
                {ocupado ? <><span className="giro" /> Abrindo…</> : pedido.validar ? 'Abrir e validar' : 'Abrir e baixar'}
              </button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  )
}
