import { useCallback, useEffect, useState } from 'react'
import { getErrorMessage } from '../context/apiClient'

export function useFetch(fetcher, intervalMs = 0, deps = []) {
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    try {
      const result = await fetcher()
      setData(result)
      setError('')
    } catch (err) {
      setError(getErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }, deps)

  useEffect(() => {
    refresh()
    if (!intervalMs) return undefined
    const timer = setInterval(refresh, intervalMs)
    return () => clearInterval(timer)
  }, [refresh, intervalMs])

  return { data, loading, error, refresh }
}
