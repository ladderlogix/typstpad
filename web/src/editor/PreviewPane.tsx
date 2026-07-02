import { useEffect, useRef, useState } from "react";

interface Props {
  svg?: string;
  compiling: boolean;
}

export default function PreviewPane({ svg, compiling }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  const [zoom, setZoom] = useState(100);

  useEffect(() => {
    if (ref.current && svg !== undefined) {
      ref.current.innerHTML = svg;
    }
  }, [svg]);

  return (
    <div className="relative flex h-full flex-col bg-gray-100">
      <div className="flex items-center justify-end gap-1 border-b border-gray-200 bg-white px-2 py-1 text-xs text-gray-500">
        {compiling && <span className="mr-auto pl-1 text-gray-400">compiling…</span>}
        <button className="rounded px-2 py-0.5 hover:bg-gray-100" onClick={() => setZoom((z) => Math.max(30, z - 10))}>
          −
        </button>
        <span className="w-10 text-center">{zoom}%</span>
        <button className="rounded px-2 py-0.5 hover:bg-gray-100" onClick={() => setZoom((z) => Math.min(300, z + 10))}>
          +
        </button>
        <button className="rounded px-2 py-0.5 hover:bg-gray-100" onClick={() => setZoom(100)}>
          reset
        </button>
      </div>
      <div className="flex-1 overflow-auto p-4">
        <div
          ref={ref}
          className="tp-preview mx-auto origin-top bg-white shadow"
          style={{ width: `${zoom}%` }}
        />
        {svg === undefined && (
          <p className="mt-12 text-center text-sm text-gray-400">
            {compiling ? "Compiling preview…" : "The preview appears here once the document compiles."}
          </p>
        )}
      </div>
    </div>
  );
}
