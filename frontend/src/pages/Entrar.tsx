import { useState } from 'react'
import { api, mascaraCPF } from '../api'
import Icone from '../components/Icone'

const passos = [
  ['01', 'Par de chaves RSA-3072', 'Gerado no servidor no momento do cadastro.'],
  ['02', 'Certificado X.509', 'Emitido pela AC ACME Assinatura Digital com o seu nome e CPF.'],
  ['03', 'Chave privada selada', 'Cifrada com AES-256-GCM; a chave de cifragem sai da sua senha via Argon2id.'],
  ['04', 'Assinatura PAdES', 'SHA-256 do PDF, assinado com RSA e embutido em um envelope CMS.'],
]

export default function Entrar({ onEntrou }: { onEntrou: () => Promise<void> }) {
  const [modo, setModo] = useState<'entrar' | 'cadastro'>('entrar')
  const [nome, setNome] = useState('')
  const [cpf, setCpf] = useState('')
  const [senha, setSenha] = useState('')
  const [senha2, setSenha2] = useState('')
  const [erro, setErro] = useState('')
  const [ocupado, setOcupado] = useState(false)

  const enviar = async (e: React.FormEvent) => {
    e.preventDefault()
    setErro('')
    if (modo === 'cadastro' && senha !== senha2) {
      setErro('As senhas não conferem.')
      return
    }
    setOcupado(true)
    try {
      if (modo === 'entrar') await api.login(cpf, senha)
      else await api.cadastro(nome, cpf, senha)
      await onEntrou()
    } catch (err) {
      setErro((err as Error).message)
    } finally {
      setOcupado(false)
    }
  }

  return (
    <div className="entrar">
      <section className="entrar-hero">
        <span className="chip lime revelar"><span className="ponto" /> Infraestrutura de chaves públicas · ACME</span>
        <h1 className="display revelar" style={{ '--i': 1 } as React.CSSProperties}>
          Assine documentos com <em>criptografia</em> de verdade.
        </h1>
        <p className="revelar" style={{ '--i': 2 } as React.CSSProperties}>
          Envie um PDF, escolha onde o carimbo aparece e assine com o seu certificado digital, como na assinatura do
          gov.br. Qualquer pessoa pode validar o documento depois.
        </p>
        <ol className="passos-cripto revelar" style={{ '--i': 3 } as React.CSSProperties}>
          {passos.map(([n, t, d]) => (
            <li key={n}>
              <span className="num">{n}</span>
              <div>
                <b>{t}</b>
                <p>{d}</p>
              </div>
            </li>
          ))}
        </ol>
      </section>

      <section className="painel entrar-form revelar" style={{ '--i': 2 } as React.CSSProperties}>
        <div className="abas" role="tablist">
          <button role="tab" aria-selected={modo === 'entrar'} className={modo === 'entrar' ? 'ativa' : ''} onClick={() => { setModo('entrar'); setErro('') }}>
            Entrar
          </button>
          <button role="tab" aria-selected={modo === 'cadastro'} className={modo === 'cadastro' ? 'ativa' : ''} onClick={() => { setModo('cadastro'); setErro('') }}>
            Criar conta
          </button>
        </div>
        <form onSubmit={enviar}>
          {modo === 'cadastro' && (
            <label>
              <span>Nome completo</span>
              <input value={nome} onChange={(e) => setNome(e.target.value)} autoComplete="name" required minLength={3} placeholder="Como vai aparecer no certificado" />
            </label>
          )}
          <label>
            <span>CPF</span>
            <input value={cpf} onChange={(e) => setCpf(mascaraCPF(e.target.value))} inputMode="numeric" placeholder="000.000.000-00" autoComplete="username" required />
          </label>
          <label>
            <span>Senha</span>
            <input
              type="password"
              value={senha}
              onChange={(e) => setSenha(e.target.value)}
              autoComplete={modo === 'entrar' ? 'current-password' : 'new-password'}
              required
              minLength={modo === 'cadastro' ? 8 : undefined}
              placeholder={modo === 'cadastro' ? 'Mínimo de 8 caracteres' : ''}
            />
          </label>
          {modo === 'cadastro' && (
            <label>
              <span>Confirme a senha</span>
              <input type="password" value={senha2} onChange={(e) => setSenha2(e.target.value)} autoComplete="new-password" required />
            </label>
          )}
          {modo === 'cadastro' && (
            <div className="nota">
              <Icone nome="cadeado" />
              <span>A senha protege a sua chave privada. Se você esquecer, a chave não pode ser recuperada, nem pelo servidor.</span>
            </div>
          )}
          {erro && <p className="erro" role="alert">{erro}</p>}
          <button className="primario grande largo" disabled={ocupado} style={{ marginTop: 'var(--e2)' }}>
            {ocupado ? (
              <><span className="giro" /> {modo === 'cadastro' ? 'Gerando chaves e certificado…' : 'Entrando…'}</>
            ) : modo === 'entrar' ? 'Entrar' : 'Criar conta e emitir certificado'}
          </button>
        </form>
        <p className="dica-cpf">CPFs de teste: 529.982.247-25 · 111.444.777-35</p>
      </section>
    </div>
  )
}
