/// <reference lib="webworker" />
// Typst WASM compile worker: holds the project's virtual filesystem and
// produces SVG previews / PDFs / diagnostics off the main thread.
import { $typst, TypstSnippet } from "@myriaddreamin/typst.ts/contrib/snippet";
import { MemoryAccessModel } from "@myriaddreamin/typst.ts/fs/memory";
import { preloadFontAssets } from "@myriaddreamin/typst.ts/options.init";
import { CompileFormatEnum } from "@myriaddreamin/typst.ts/compiler";
import compilerWasm from "@myriaddreamin/typst-ts-web-compiler/wasm?url";
import rendererWasm from "@myriaddreamin/typst-ts-renderer/wasm?url";

export interface WorkerDiagnostic {
  severity: "error" | "warning";
  file: string;
  line: number;
  col: number;
  message: string;
}

type InMsg =
  | { type: "init"; origin: string }
  | { type: "sync"; files?: Record<string, string>; assets?: Record<string, ArrayBuffer> }
  | { type: "compile"; seq: number; mainPath: string }
  | { type: "pdf"; seq: number; mainPath: string };

type OutMsg =
  | { type: "ready" }
  | { type: "result"; seq: number; svg?: string; diagnostics: WorkerDiagnostic[] }
  | { type: "pdf"; seq: number; data?: ArrayBuffer; diagnostics: WorkerDiagnostic[] }
  | { type: "fatal"; message: string };

const post = (msg: OutMsg, transfer: Transferable[] = []) =>
  (self as unknown as DedicatedWorkerGlobalScope).postMessage(msg, transfer);

let origin = self.location.origin;
let initialized: Promise<void> | null = null;

function init(): Promise<void> {
  if (initialized) return initialized;
  initialized = (async () => {
    $typst.setCompilerInitOptions({
      getModule: () => compilerWasm,
      beforeBuild: [
        // Fonts come from our own server (disk-cached proxy) so browsers
        // never need direct internet access.
        preloadFontAssets({
          assets: ["text", "cjk", "emoji"],
          assetUrlPrefix: {
            text: `${origin}/api/typst/fonts/typst-assets/`,
            _: `${origin}/api/typst/fonts/typst-dev-assets/`,
          },
        }),
      ],
    });
    $typst.setRendererInitOptions({ getModule: () => rendererWasm });

    // Typst Universe package downloads go through the backend proxy (CORS +
    // caching). Sync XHR is fine in a worker.
    const mem = new MemoryAccessModel();
    $typst.use(
      TypstSnippet.withAccessModel(mem),
      TypstSnippet.fetchPackageBy(mem, (_spec, defaultHttpUrl) => {
        try {
          const url = defaultHttpUrl.replace("https://packages.typst.org/", `${origin}/api/typst/packages/`);
          const xhr = new XMLHttpRequest();
          xhr.open("GET", url, false);
          xhr.responseType = "arraybuffer";
          xhr.send();
          if (xhr.status === 200 && xhr.response) {
            return new Uint8Array(xhr.response as ArrayBuffer);
          }
        } catch (err) {
          console.error("package fetch failed", err);
        }
        return undefined;
      })
    );
    // Force compiler + renderer initialization now.
    await $typst.getCompiler();
    await $typst.getRenderer();
  })();
  return initialized;
}

function parseRange(range: string): { line: number; col: number } {
  // "2:9-3:15" (1-based in typst.ts full diagnostics)
  const m = /^(\d+):(\d+)/.exec(range ?? "");
  if (!m) return { line: 1, col: 1 };
  return { line: parseInt(m[1], 10), col: parseInt(m[2], 10) };
}

function toDiagnostics(raw: any[] | undefined): WorkerDiagnostic[] {
  if (!raw) return [];
  return raw.map((d) => {
    const { line, col } = parseRange(d.range);
    return {
      severity: d.severity === "warning" ? "warning" : "error",
      file: String(d.path ?? "").replace(/^\//, ""),
      line,
      col,
      message: String(d.message ?? ""),
    };
  });
}

async function handleSync(msg: Extract<InMsg, { type: "sync" }>) {
  await init();
  for (const [path, content] of Object.entries(msg.files ?? {})) {
    await $typst.addSource("/" + path, content);
  }
  for (const [path, bytes] of Object.entries(msg.assets ?? {})) {
    await $typst.mapShadow("/" + path, new Uint8Array(bytes));
  }
}

async function handleCompile(msg: Extract<InMsg, { type: "compile" }>) {
  await init();
  const compiler = await $typst.getCompiler();
  const result = await compiler.compile({
    mainFilePath: "/" + msg.mainPath,
    format: CompileFormatEnum.vector,
    diagnostics: "full",
  });
  const diagnostics = toDiagnostics(result.diagnostics as any[]);
  if (!result.result) {
    post({ type: "result", seq: msg.seq, diagnostics });
    return;
  }
  const svg = await $typst.svg({ vectorData: result.result });
  post({ type: "result", seq: msg.seq, svg, diagnostics });
}

async function handlePdf(msg: Extract<InMsg, { type: "pdf" }>) {
  await init();
  const compiler = await $typst.getCompiler();
  const result = await compiler.compile({
    mainFilePath: "/" + msg.mainPath,
    format: CompileFormatEnum.pdf,
    diagnostics: "full",
  });
  const diagnostics = toDiagnostics(result.diagnostics as any[]);
  const data = result.result?.buffer as ArrayBuffer | undefined;
  post({ type: "pdf", seq: msg.seq, data, diagnostics }, data ? [data] : []);
}

self.onmessage = async (ev: MessageEvent<InMsg>) => {
  const msg = ev.data;
  try {
    switch (msg.type) {
      case "init":
        origin = msg.origin;
        await init();
        post({ type: "ready" });
        break;
      case "sync":
        await handleSync(msg);
        break;
      case "compile":
        await handleCompile(msg);
        break;
      case "pdf":
        await handlePdf(msg);
        break;
    }
  } catch (err) {
    console.error("typst worker error", err);
    if (msg.type === "compile") {
      post({
        type: "result",
        seq: msg.seq,
        diagnostics: [{ severity: "error", file: "", line: 1, col: 1, message: String(err) }],
      });
    } else if (msg.type === "pdf") {
      post({
        type: "pdf",
        seq: msg.seq,
        diagnostics: [{ severity: "error", file: "", line: 1, col: 1, message: String(err) }],
      });
    } else {
      post({ type: "fatal", message: String(err) });
    }
  }
};
