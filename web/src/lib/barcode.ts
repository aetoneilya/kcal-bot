/**
 * Reads a product barcode (EAN-13, EAN-8, UPC) off a photo.
 *
 * From a still photo rather than a live camera: Telegram's iOS web view
 * does not reliably give a page the camera stream, while a file input with
 * capture works everywhere. The browser's own BarcodeDetector is used where
 * it exists (Android); elsewhere zxing, loaded only on first use.
 */

const MAX_SIDE = 1600;

export async function decodeBarcode(blob: Blob): Promise<string | null> {
  const bitmap = await createImageBitmap(blob);
  try {
    const native = await detectNative(bitmap);
    if (native) return native;
    return await detectZxing(bitmap);
  } finally {
    bitmap.close();
  }
}

type Detector = { detect(source: ImageBitmap): Promise<{ rawValue: string }[]> };
type DetectorClass = new (options: { formats: string[] }) => Detector;

async function detectNative(bitmap: ImageBitmap): Promise<string | null> {
  const Native = (window as unknown as { BarcodeDetector?: DetectorClass }).BarcodeDetector;
  if (!Native) return null;
  try {
    const found = await new Native({ formats: ["ean_13", "ean_8", "upc_a", "upc_e"] }).detect(bitmap);
    return found[0]?.rawValue ?? null;
  } catch {
    return null;
  }
}

async function detectZxing(bitmap: ImageBitmap): Promise<string | null> {
  const zx = await import("@zxing/library");
  const hints = new Map<import("@zxing/library").DecodeHintType, unknown>([
    [zx.DecodeHintType.POSSIBLE_FORMATS, [zx.BarcodeFormat.EAN_13, zx.BarcodeFormat.EAN_8, zx.BarcodeFormat.UPC_A, zx.BarcodeFormat.UPC_E]],
    [zx.DecodeHintType.TRY_HARDER, true],
  ]);
  const reader = new zx.MultiFormatReader();
  // A barcode photographed sideways is common; try both orientations.
  for (const rotate of [false, true]) {
    const { data, width, height } = luminance(bitmap, rotate);
    try {
      const source = new zx.RGBLuminanceSource(data, width, height);
      return reader.decode(new zx.BinaryBitmap(new zx.HybridBinarizer(source)), hints).getText();
    } catch {
      // NotFoundException: try the other orientation.
    }
  }
  return null;
}

/** Grey levels of the photo, scaled down and optionally turned 90°. */
function luminance(bitmap: ImageBitmap, rotate: boolean) {
  const scale = Math.min(1, MAX_SIDE / Math.max(bitmap.width, bitmap.height));
  const w = Math.round(bitmap.width * scale);
  const h = Math.round(bitmap.height * scale);
  const [width, height] = rotate ? [h, w] : [w, h];
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext("2d", { willReadFrequently: true })!;
  if (rotate) {
    ctx.translate(width, 0);
    ctx.rotate(Math.PI / 2);
  }
  ctx.drawImage(bitmap, 0, 0, w, h);
  const rgba = ctx.getImageData(0, 0, width, height).data;
  const data = new Uint8ClampedArray(width * height);
  for (let i = 0, j = 0; i < data.length; i++, j += 4) {
    data[i] = (rgba[j] * 299 + rgba[j + 1] * 587 + rgba[j + 2] * 114) / 1000;
  }
  return { data, width, height };
}
