// Main-thread RPC wrapper around the Typst WASM worker.
import type { WorkerDiagnostic } from "./worker";

export type { WorkerDiagnostic };

export interface CompileOutput {
  svg?: string;
  diagnostics: WorkerDiagnostic[];
}

export class TypstClient {
  private worker: Worker;
  private seq = 0;
  private pending = new Map<number, (msg: any) => void>();
  ready: Promise<void>;
  onFatal: ((message: string) => void) | null = null;

  constructor() {
    this.worker = new Worker(new URL("./worker.ts", import.meta.url), { type: "module" });
    let resolveReady!: () => void;
    this.ready = new Promise((r) => (resolveReady = r));
    this.worker.onmessage = (ev) => {
      const msg = ev.data;
      if (msg.type === "ready") {
        resolveReady();
        return;
      }
      if (msg.type === "fatal") {
        this.onFatal?.(msg.message);
        return;
      }
      const cb = this.pending.get(msg.seq);
      if (cb) {
        this.pending.delete(msg.seq);
        cb(msg);
      }
    };
    this.worker.postMessage({ type: "init", origin: window.location.origin });
  }

  sync(files?: Record<string, string>, assets?: Record<string, ArrayBuffer>) {
    this.worker.postMessage(
      { type: "sync", files, assets },
      assets ? Object.values(assets) : []
    );
  }

  compile(mainPath: string): Promise<CompileOutput> {
    const seq = ++this.seq;
    return new Promise((resolve) => {
      this.pending.set(seq, (msg) => resolve({ svg: msg.svg, diagnostics: msg.diagnostics }));
      this.worker.postMessage({ type: "compile", seq, mainPath });
    });
  }

  pdf(mainPath: string): Promise<{ data?: ArrayBuffer; diagnostics: WorkerDiagnostic[] }> {
    const seq = ++this.seq;
    return new Promise((resolve) => {
      this.pending.set(seq, (msg) => resolve({ data: msg.data, diagnostics: msg.diagnostics }));
      this.worker.postMessage({ type: "pdf", seq, mainPath });
    });
  }

  destroy() {
    this.worker.terminate();
  }
}
