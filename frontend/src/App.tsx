import { useCallback, useEffect, useState } from 'react'
import { api, type Eu } from './api'
import CampoNeural from './components/neural/CampoNeural'
import Icone, { type NomeIcone } from './components/Icone'
import Entrar from './pages/Entrar'
import Assinar from './pages/Assinar'
import Validar from './pages/Validar'
import Documentos from './pages/Documentos'
import MeuCertificado from './pages/MeuCertificado'
import AutoridadeCertificadora from './pages/AutoridadeCertificadora'

// Roteador mínimo baseado em history.pushState (sem dependências).
export function navegar(caminho: string) {
  if (location.pathname !== caminho) history.pushState(null, '', caminho)
  dispatchEvent(new PopStateEvent('popstate'))
  scrollTo({ top: 0 })
}

function useCaminho() {
  const [p, setP] = useState(location.pathname)
  useEffect(() => {
    const f = () => setP(location.pathname)
    addEventListener('popstate', f)
    return () => removeEventListener('popstate', f)
  }, [])
  return p
}

const menu: { caminho: string; rotulo: string; icone: NomeIcone; privado: boolean }[] = [
  { caminho: '/assinar', rotulo: 'Assinar', icone: 'caneta', privado: true },
  { caminho: '/validar', rotulo: 'Validar', icone: 'escudo', privado: false },
  { caminho: '/documentos', rotulo: 'Documentos', icone: 'lista', privado: true },
  { caminho: '/certificado', rotulo: 'Certificado', icone: 'certificado', privado: true },
  { caminho: '/ac', rotulo: 'AC ACME', icone: 'rede', privado: false },
]

export default function App() {
  const caminho = useCaminho()
  const [eu, setEu] = useState<Eu | null | undefined>(undefined)
  const [paraValidar, setParaValidar] = useState<{ blob: Blob; nome: string } | null>(null)

  const recarrega = useCallback(async () => setEu(await api.eu()), [])
  useEffect(() => {
    recarrega().catch(() => setEu(null))
  }, [recarrega])

  const validar = (blob: Blob, nome: string) => {
    setParaValidar({ blob, nome })
    navegar('/validar')
  }

  const publica = caminho === '/validar' || caminho === '/ac'
  let pagina
  if (eu === undefined) {
    pagina = <div className="carregando"><span className="giro" /> Carregando…</div>
  } else if (!eu && !publica) {
    pagina = <Entrar onEntrou={recarrega} />
  } else if (caminho === '/validar') {
    pagina = <Validar inicial={paraValidar} onConsumido={() => setParaValidar(null)} />
  } else if (caminho === '/documentos') {
    pagina = <Documentos onValidar={validar} />
  } else if (caminho === '/certificado' && eu) {
    pagina = <MeuCertificado eu={eu} onMudou={recarrega} />
  } else if (caminho === '/ac') {
    pagina = <AutoridadeCertificadora />
  } else if (eu) {
    pagina = <Assinar eu={eu} onValidar={validar} />
  }

  const ativo = menu.find((m) => m.caminho === caminho)?.caminho ?? (eu ? '/assinar' : '')

  return (
    <>
      <CampoNeural />
      <div className="casca">
        <header className="topo">
          <div className="topo-interno">
            <a className="marca" href="/" onClick={(e) => { e.preventDefault(); navegar('/assinar') }}>
              <span className="marca-icone"><Icone nome="check" tamanho={16} /></span>
              <span>LCR <b>Assina</b></span>
            </a>
            <nav>
              {menu
                .filter((m) => eu || !m.privado)
                .map((m) => (
                  <a
                    key={m.caminho}
                    href={m.caminho}
                    className={ativo === m.caminho ? 'ativo' : ''}
                    onClick={(e) => { e.preventDefault(); navegar(m.caminho) }}
                  >
                    <Icone nome={m.icone} tamanho={16} />
                    {m.rotulo}
                  </a>
                ))}
            </nav>
            <div className="usuario">
              {eu ? (
                <>
                  <span className="avatar" title={eu.nome}>{eu.nome.slice(0, 1).toUpperCase()}</span>
                  <span className="sutil">{eu.nome.split(' ')[0]}</span>
                  <button
                    className="link"
                    title="Sair"
                    onClick={async () => { await api.logout(); setEu(null); navegar('/') }}
                  >
                    <Icone nome="sair" tamanho={16} />
                  </button>
                </>
              ) : (
                eu === null && publica && <button className="primario" onClick={() => navegar('/assinar')}>Entrar</button>
              )}
            </div>
          </div>
        </header>
        <main>{pagina}</main>
        <footer>
          <div>
            <span>LCR Assina · PKI própria · PAdES · RSA-3072 + SHA-256</span>
            <span>Projeto didático de criptografia, sem validade jurídica</span>
          </div>
        </footer>
      </div>
    </>
  )
}
