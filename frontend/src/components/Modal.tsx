import { useEffect, type ReactNode } from 'react'
import Icone from './Icone'

export default function Modal({ titulo, children, onFechar }: { titulo: string; children: ReactNode; onFechar: () => void }) {
  useEffect(() => {
    const esc = (e: KeyboardEvent) => e.key === 'Escape' && onFechar()
    addEventListener('keydown', esc)
    return () => removeEventListener('keydown', esc)
  }, [onFechar])

  return (
    <div className="modal-fundo" onMouseDown={(e) => e.target === e.currentTarget && onFechar()}>
      <div className="modal" role="dialog" aria-modal="true" aria-label={titulo}>
        <div className="modal-topo">
          <h3 className="subtitulo">{titulo}</h3>
          <button className="fechar" onClick={onFechar} aria-label="Fechar"><Icone nome="fechar" /></button>
        </div>
        {children}
      </div>
    </div>
  )
}
