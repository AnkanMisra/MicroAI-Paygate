const BASE64_RE = /^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/;

export function decodeBase64ToUtf8(value: string): string {
  if (value.length === 0 || value.trim() !== value || !BASE64_RE.test(value)) {
    throw new Error("invalid base64");
  }

  if (typeof globalThis.atob !== "function") {
    throw new Error("base64 decoding is unavailable in this runtime");
  }

  const binary = globalThis.atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i);
  }
  return new TextDecoder().decode(bytes);
}

export function encodeUtf8ToBase64(value: string): string {
  if (typeof globalThis.btoa !== "function") {
    throw new Error("base64 encoding is unavailable in this runtime");
  }

  const bytes = new TextEncoder().encode(value);
  let binary = "";
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return globalThis.btoa(binary);
}
