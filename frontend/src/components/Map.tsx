import { useRef, useEffect } from 'react'
import mapboxgl from 'mapbox-gl'

interface MapProps {
  token: string
}

export default function Map({ token }: MapProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<mapboxgl.Map | null>(null)

  useEffect(() => {
    if (!containerRef.current || !token) return

    mapboxgl.accessToken = token
    const map = new mapboxgl.Map({
      container: containerRef.current,
      style: 'mapbox://styles/mapbox/dark-v11',
      center: [-74.006, 40.7128],
      zoom: 10,
    })
    mapRef.current = map
    return () => {
      map.remove()
      mapRef.current = null
    }
  }, [token])

  return <div ref={containerRef} style={{ width: '100%', height: '100%' }} />
}
