type RouteName = string | symbol;

interface RouteLike {
  name?: RouteName | null;
}

interface RouteRedirect {
  path: string;
  replace: true;
}

export type ProtectedRouteDecision = true | RouteRedirect;

export function resolveProtectedRouteDecision(
  to: RouteLike,
  hasRoute: (name: RouteName) => boolean,
  hasPermission: boolean,
): ProtectedRouteDecision {
  if (to.name && hasRoute(to.name)) {
    return hasPermission ? true : { path: '/result/403', replace: true };
  }

  return { path: '/result/404', replace: true };
}
