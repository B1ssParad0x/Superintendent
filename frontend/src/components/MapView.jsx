import { useEffect, useRef } from 'react'
import mapboxgl from 'mapbox-gl'
import 'mapbox-gl/dist/mapbox-gl.css'

function featureCollection(points) {
  return {
    type: 'FeatureCollection',
    features: points
      .filter((p) => p?.loc?.lon != null && p?.loc?.lat != null)
      .map((p) => ({
        type: 'Feature',
        geometry: { type: 'Point', coordinates: [p.loc.lon, p.loc.lat] },
        properties: { node_id: p.node_id, ts: p.ts },
      })),
  }
}

export default function MapView({ telemetry = [], nodes = [] }) {
  const mapRef = useRef(null)
  const containerRef = useRef(null)

  useEffect(() => {
    if (!containerRef.current || mapRef.current) return
    const token = import.meta.env.VITE_MAPBOX_TOKEN
    if (!token) return
    mapboxgl.accessToken = token
    mapRef.current = new mapboxgl.Map({
      container: containerRef.current,
      style: 'mapbox://styles/mapbox/dark-v11',
      center: [-74.006, 40.7128],
      zoom: 10,
    })

    mapRef.current.on('load', () => {
      const map = mapRef.current
      map.addSource('telemetry-source', { type: 'geojson', data: featureCollection(telemetry) })
      map.addLayer({
        id: 'telemetry-heat',
        type: 'heatmap',
        source: 'telemetry-source',
        paint: {
          'heatmap-intensity': 1.05,
          'heatmap-radius': 22,
          'heatmap-opacity': 0.85,
          'heatmap-color': ['interpolate', ['linear'], ['heatmap-density'], 0, 'rgba(0,0,0,0)', 0.5, '#7f1d1d', 1, '#ef4444'],
        },
      })
      map.addSource('node-source', { type: 'geojson', data: featureCollection(nodes) })
      map.addLayer({
        id: 'node-points',
        type: 'circle',
        source: 'node-source',
        paint: {
          'circle-color': '#ffffff',
          'circle-radius': 4,
          'circle-stroke-width': 1,
          'circle-stroke-color': '#D2042D',
        },
      })
    })

    return () => {
      if (mapRef.current) {
        mapRef.current.remove()
        mapRef.current = null
      }
    }
  }, [])

  useEffect(() => {
    const map = mapRef.current
    if (!map || !map.isStyleLoaded()) return
    const telemetrySource = map.getSource('telemetry-source')
    const nodeSource = map.getSource('node-source')
    if (telemetrySource?.setData) telemetrySource.setData(featureCollection(telemetry))
    if (nodeSource?.setData) nodeSource.setData(featureCollection(nodes))
  }, [telemetry, nodes])

  if (!import.meta.env.VITE_MAPBOX_TOKEN) {
    return <div className="flex h-full items-center justify-center text-sm text-zinc-500">Set `VITE_MAPBOX_TOKEN` to enable map.</div>
  }

  return <div ref={containerRef} className="h-full min-h-[340px] w-full rounded-xl border border-zinc-800" />
}
