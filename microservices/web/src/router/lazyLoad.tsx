import { lazy, Suspense, type ComponentType } from 'react'
import RouteFallback from './RouteFallback'

export type RouteStyleLoader = () => Promise<unknown>

export function lazyLoad(
  factory: () => Promise<{ default: ComponentType }>,
  styles: RouteStyleLoader[] = [],
) {
  const Comp = lazy(
    styles.length
      ? async () => {
          const [page] = await Promise.all([factory(), ...styles.map((loadStyle) => loadStyle())])
          return page
        }
      : factory,
  )
  return (
    <Suspense fallback={<RouteFallback />}>
      <Comp />
    </Suspense>
  )
}
