export async function initializeHomeState(
  loadLibrary: () => Promise<void>,
  refreshHome: () => Promise<void>,
): Promise<void> {
  await loadLibrary()
  await refreshHome()
}
