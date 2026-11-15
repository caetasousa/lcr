import { useRef, useState } from 'react'
import Icone from './Icone'

export default function SeletorArquivo({ onArquivo, texto }: { onArquivo: (f: File) => void; texto: string }) {
  const input = useRef<HTMLInputElement>(null)
  const [arrastando, setArrastando] = useState(false)
  const [erro, setErro] = useState('')

  const recebe = (f?: File) => {
    if (!f) return
    if (f.type !== 'application/pdf' && !f.name.toLowerCase().endsWith('.pdf')) {
      setErro('Selecione um arquivo PDF.')
      return
    }
    if (f.size > 20 * 1024 * 1024) {
      setErro('O arquivo passa de 20 MB.')
      return
    }
    setErro('')
    onArquivo(f)
  }

  return (
    <>
      <div
        role="button"
        tabIndex={0}
        className={'soltar' + (arrastando ? ' arrastando' : '')}
        onClick={() => input.current?.click()}
        onKeyDown={(e) => (e.key === 'Enter' || e.key === ' ') && input.current?.click()}
        onDragOver={(e) => { e.preventDefault(); setArrastando(true) }}
        onDragLeave={() => setArrastando(false)}
        onDrop={(e) => { e.preventDefault(); setArrastando(false); recebe(e.dataTransfer.files[0]) }}
      >
        <div className="icone-grande"><Icone nome="enviar" tamanho={24} /></div>
        <p className="subtitulo">{texto}</p>
        <p className="sutil">Arraste o PDF para cá ou <span className="destaque">clique para escolher</span>. Até 20 MB.</p>
        <input
          ref={input}
          type="file"
          accept="application/pdf,.pdf"
          hidden
          onChange={(e) => { recebe(e.target.files?.[0]); e.target.value = '' }}
        />
      </div>
      {erro && <p className="erro">{erro}</p>}
    </>
  )
}
