import { useRef, useEffect, useState, useCallback } from 'react'
import mapboxgl from 'mapbox-gl'
import { getTelemetry, TelemetryPoint } from '../api'

interface MapProps {
  token: string
}

function toGeoJSON(points: TelemetryPoint[]): GeoJSON.FeatureCollection {
  const features: GeoJSON.Feature[] = points
    .filter((p) => p.loc?.lat != null && p.loc?.lon != null)
    .map((p) => ({
      type: 'Feature' as const,
      geometry: {
        type: 'Point' as const,
        coordinates: [p.loc.lon, p.loc.lat],
      },
      properties: { node_id: p.node_id },
    }))
  return { type: 'FeatureCollection', features }
}

export default function Map({ token }: MapProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const mapRef = useRef<mapboxgl.Map | null>(null)
  const sourceRef = useRef<string | null>(null)
  const [telemetry, setTelemetry] = useState<TelemetryPoint[]>([])

  const refreshTelemetry = useCallback(async () => {
    try {
      const data = await getTelemetry()
      setTelemetry(data)
    } catch {
      // ignore
    }
  }, [])

  useEffect(() => {
    refreshTelemetry()
    const t = setInterval(refreshTelemetry, 30000)
    return () => clearInterval(t)
  }, [refreshTelemetry])

  useEffect(() => {
    if (!containerRef.current || !token) return

    mapboxgl.accessToken = token
    const map = new mapboxgl.Map({
      container: containerRef.current,
      style: 'mapbox://styles/mapbox/dark-v11',
      center: [-74.006, 40.7128],
      zoom: 10,
    })

    map.on('load', () => {
      map.addSource('telemetry', {
        type: 'geojson',
        data: toGeoJSON(telemetry),
      })
      sourceRef.current = 'telemetry'
      map.addLayer({
        id: 'telemetry-heatmap',
        type: 'heatmap',
        source: 'telemetry',
        maxzoom: 15,
        paint: {
          'heatmap-weight': 1,
          'heatmap-intensity': 1,
          'heatmap-color': [
            'interpolate',
            ['linear'],
            ['heatmap-density'],
            0, 'rgba(33,102,172,0)',
            0.2, 'rgb(103,169,207)',
            0.4, 'rgb(209,229,240)',
            0.6, 'rgb(253,219,199)',
            0.8, 'rgb(239,138,98)',
            1, 'rgb(178,24,43)',
          ],
          'heatmap-radius': 20,
        },
      })
    })
    mapRef.current = map
    return () => {
      map.remove()
      mapRef.current = null
      sourceRef.current = null
    }
  }, [token])

  useEffect(() => {
    const map = mapRef.current
    const source = sourceRef.current
    if (!map || !source) return
    const s = map.getSource(source)
    if (s && 'setData' in s) {
      ;(s as mapboxgl.GeoJSONSource).setData(toGeoJSON(telemetry))
    }
  }, [telemetry])

  return <div ref={containerRef} style={{ width: '100%', height: '100%' }} />
}
