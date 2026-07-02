import { useEffect, useImperativeHandle, useRef, useState, forwardRef } from "react";

export interface PreviewHandle {
  /** Scroll the preview so the given document fraction is centered. */
  scrollToFraction: (fraction: number) => void;
}

interface Props {
  svg?: string;
  compiling: boolean;
  /** Fired when the user clicks in the preview: fraction of document height. */
  onJumpToFraction?: (fraction: number) => void;
  syncEnabled: boolean;
  onToggleSync: () => void;
}

const PreviewPane = forwardRef<PreviewHandle, Props>(function PreviewPane(
  { svg, compiling, onJumpToFraction, syncEnabled, onToggleSync },
  ref
) {
  const scrollerRef = useRef<HTMLDivElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const [zoom, setZoom] = useState(100);

  useEffect(() => {
    if (contentRef.current && svg !== undefined) {
      contentRef.current.innerHTML = svg;
    }
  }, [svg]);

  useImperativeHandle(ref, () => ({
    scrollToFraction(fraction: number) {
      const scroller = scrollerRef.current;
      const content = contentRef.current;
      if (!scroller || !content) return;
      const target = content.offsetTop + fraction * content.scrollHeight - scroller.clientHeight / 2;
      scroller.scrollTo({ top: Math.max(0, target), behavior: "smooth" });
    },
  }));

  function handleClick(e: React.MouseEvent) {
    if (!onJumpToFraction || !contentRef.current || !scrollerRef.current) return;
    // Double-click jumps the editor to the corresponding place (approximate).
    if (e.detail !== 2) return;
    const rect = contentRef.current.getBoundingClientRect();
    const fraction = (e.clientY - rect.top) / rect.height;
    if (fraction >= 0 && fraction <= 1) onJumpToFraction(fraction);
  }

  return (
    <div className="relative flex h-full flex-col bg-gray-100">
      <div className="flex items-center justify-end gap-1 border-b border-gray-200 bg-white px-2 py-1 text-xs text-gray-500">
        {compiling && <span className="mr-auto pl-1 text-gray-400">compiling…</span>}
        <button
          className={`rounded px-2 py-0.5 ${syncEnabled ? "bg-indigo-100 text-indigo-700" : "hover:bg-gray-100"}`}
          onClick={onToggleSync}
          title="Follow the editor cursor (approximate). Double-click the preview to jump the editor."
        >
          sync
        </button>
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
      <div ref={scrollerRef} className="flex-1 overflow-auto p-4" onClick={handleClick}>
        <div
          ref={contentRef}
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
});

export default PreviewPane;
