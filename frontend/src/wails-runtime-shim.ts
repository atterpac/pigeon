export const Call = { ByID: (id: number) => Promise.reject(new Error(`Wails runtime unavailable for call ${id}`)) }
export const CancellablePromise = Promise
export const Create = {
  Any: (value: unknown) => value,
  Array: (create: (value: unknown) => unknown) => (value: unknown) => (Array.isArray(value) ? value.map(create) : []),
  Nullable: (create: (value: unknown) => unknown) => (value: unknown) => (value == null ? null : create(value)),
  ByteSlice: (value: unknown) => value,
}
