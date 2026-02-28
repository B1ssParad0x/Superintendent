export default function AudioPlayer({ src, className = '' }) {
  if (!src) {
    return <p className={`text-xs text-zinc-500 ${className}`}>No audio advisory</p>
  }
  return (
    <audio
      controls
      preload="none"
      className={`w-full rounded-md border border-zinc-800 bg-zinc-950/80 p-1 ${className}`}
      src={src}
    />
  )
}
