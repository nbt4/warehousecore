import { useState, useEffect, useRef, useCallback } from 'react';
import {
  Trash2, Download, Printer, QrCode, Barcode, Type, Save,
  Image as ImageIcon, Lock, Unlock, Grid3x3, Eye, EyeOff,
} from 'lucide-react';
import { labelsApi, devicesApi, casesApi } from '../lib/api';
import type { LabelTemplate, LabelElement, Device, CaseSummary } from '../lib/api';
import JSZip from 'jszip';
import './LabelDesignerPage.css';

/* ── Types ──────────────────────────────────────────────────────────── */

interface DesignElement extends LabelElement {
  id: string;
  image_data?: string;
  aspect_ratio_locked?: boolean;
}

type ResizeHandle = 'nw' | 'n' | 'ne' | 'e' | 'se' | 's' | 'sw' | 'w';

interface DragState {
  type: 'move' | 'resize';
  handle?: ResizeHandle;
  elementId: string;
  startX: number;
  startY: number;
  origX: number;
  origY: number;
  origW: number;
  origH: number;
  origRatio: number;
}

/* ── Constants ──────────────────────────────────────────────────────── */

const ZEBRA_PRESETS = [
  { label: '62 × 29 mm (Standard)',         w: 62,    h: 29    },
  { label: '57 × 32 mm  (2.25 × 1.25")',    w: 57.2,  h: 31.8  },
  { label: '101 × 51 mm (4 × 2")',           w: 101.6, h: 50.8  },
  { label: '101 × 25 mm (4 × 1")',           w: 101.6, h: 25.4  },
  { label: '51 × 25 mm  (2 × 1")',           w: 50.8,  h: 25.4  },
  { label: '38 × 25 mm  (1.5 × 1")',         w: 38.1,  h: 25.4  },
  { label: '25 × 25 mm  (1 × 1")',           w: 25.4,  h: 25.4  },
  { label: '101 × 152 mm (4 × 6" Versand)',  w: 101.6, h: 152.4 },
  { label: 'Custom',                          w: 0,     h: 0     },
];

const FIELD_OPTIONS = [
  { value: 'device_id',     label: 'Device ID'    },
  { value: 'product_name',  label: 'Produktname'  },
  { value: 'serial_number', label: 'Seriennummer' },
  { value: 'barcode',       label: 'Barcode'      },
  { value: 'zone_code',     label: 'Zonen-Code'   },
  { value: 'zone_name',     label: 'Zonenname'    },
  { value: 'status',        label: 'Status'       },
];

const EDITOR_MAX = 480;
const GRID_MM    = 1;

const HANDLES: ResizeHandle[] = ['nw', 'n', 'ne', 'w', 'e', 'sw', 's', 'se'];

const HANDLE_POS: Record<ResizeHandle, { top: string; left: string; cursor: string }> = {
  nw: { top: '-4px',             left: '-4px',             cursor: 'nw-resize' },
  n:  { top: '-4px',             left: 'calc(50% - 4px)',  cursor: 'n-resize'  },
  ne: { top: '-4px',             left: 'calc(100% - 4px)', cursor: 'ne-resize' },
  w:  { top: 'calc(50% - 4px)',  left: '-4px',             cursor: 'w-resize'  },
  e:  { top: 'calc(50% - 4px)',  left: 'calc(100% - 4px)', cursor: 'e-resize'  },
  sw: { top: 'calc(100% - 4px)', left: '-4px',             cursor: 'sw-resize' },
  s:  { top: 'calc(100% - 4px)', left: 'calc(50% - 4px)',  cursor: 's-resize'  },
  se: { top: 'calc(100% - 4px)', left: 'calc(100% - 4px)', cursor: 'se-resize' },
};

function snapVal(v: number, grid: number, enabled: boolean): number {
  if (!enabled || grid <= 0) return v;
  return Math.round(v / grid) * grid;
}

/* ── Component ──────────────────────────────────────────────────────── */

export default function LabelDesignerPage() {
  const [labelW, setLabelW]         = useState(62);
  const [labelH, setLabelH]         = useState(29);
  const [elements, setElements]     = useState<DesignElement[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [snapEnabled, setSnapEnabled]   = useState(true);
  const [showPreview, setShowPreview]   = useState(true);

  const [templateName, setTemplateName]   = useState('Neues Template');
  const [templates, setTemplates]         = useState<LabelTemplate[]>([]);
  const [currentTplId, setCurrentTplId]   = useState<number | null>(null);
  const [saving, setSaving]               = useState(false);
  const [exporting, setExporting]         = useState(false);

  const [devices, setDevices]             = useState<Device[]>([]);
  const [cases, setCases]                 = useState<CaseSummary[]>([]);
  const [previewDevice, setPreviewDevice] = useState<Device | null>(null);

  const [imgCache, setImgCache] = useState<Record<string, string>>({});

  const canvasRef    = useRef<HTMLCanvasElement>(null);
  const dragRef      = useRef<DragState | null>(null);
  const previewTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const imgTimer     = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Refs so document-level event handlers always see fresh values
  const elementsRef   = useRef(elements);
  const snapRef       = useRef(snapEnabled);
  const labelWRef     = useRef(labelW);
  const labelHRef     = useRef(labelH);
  elementsRef.current = elements;
  snapRef.current     = snapEnabled;
  labelWRef.current   = labelW;
  labelHRef.current   = labelH;

  // Scale: fit label into EDITOR_MAX × EDITOR_MAX px
  const scale = Math.min(EDITOR_MAX / labelW, EDITOR_MAX / labelH);
  const edW   = Math.round(labelW * scale);
  const edH   = Math.round(labelH * scale);

  useEffect(() => { loadDevices(); loadCases(); loadTemplates(true); }, []);

  // Debounced canvas preview
  useEffect(() => {
    if (previewTimer.current) clearTimeout(previewTimer.current);
    previewTimer.current = setTimeout(() => renderPreview(), 600);
    return () => { if (previewTimer.current) clearTimeout(previewTimer.current); };
  }, [elements, labelW, labelH, previewDevice]);

  // Debounced editor QR/barcode image fetch
  useEffect(() => {
    if (imgTimer.current) clearTimeout(imgTimer.current);
    imgTimer.current = setTimeout(async () => {
      const codes = elements.filter(e => e.type === 'qrcode' || e.type === 'barcode');
      if (codes.length === 0) return;
      const updates: Record<string, string> = {};
      await Promise.all(codes.map(async elem => {
        const demoContent = elem.content === 'device_id' ? 'DEMO-001' : elem.content;
        try {
          if (elem.type === 'qrcode') {
            const px = Math.max(60, Math.round(elem.width * Math.min(EDITOR_MAX / labelW, EDITOR_MAX / labelH)));
            const { data } = await labelsApi.generateQRCode(demoContent, px);
            updates[elem.id] = data.image_data;
          } else {
            const px = Math.max(123, Math.round(elem.width * Math.min(EDITOR_MAX / labelW, EDITOR_MAX / labelH)));
            const ph = Math.max(20, Math.round(elem.height * Math.min(EDITOR_MAX / labelW, EDITOR_MAX / labelH)));
            const { data } = await labelsApi.generateBarcode(demoContent, px, ph);
            updates[elem.id] = data.image_data;
          }
        } catch { /* keep placeholder on error */ }
      }));
      if (Object.keys(updates).length > 0)
        setImgCache(prev => ({ ...prev, ...updates }));
    }, 800);
    return () => { if (imgTimer.current) clearTimeout(imgTimer.current); };
  }, [elements.map(e => `${e.id}:${e.content}:${e.width}:${e.height}`).join(','), labelW, labelH]);

  const toast = (type: 'success' | 'error', message: string) =>
    window.dispatchEvent(new CustomEvent('toast', { detail: { type, message } }));

  /* ── Data loading ── */

  const loadDevices = async () => {
    try {
      const { data } = await devicesApi.getAll({ limit: 50000 });
      setDevices(data);
      if (data.length > 0) setPreviewDevice(data[0]);
    } catch { console.error('Failed to load devices'); }
  };

  const loadCases = async () => {
    try {
      const { data } = await casesApi.list({});
      setCases(data.cases || []);
    } catch { console.error('Failed to load cases'); }
  };

  const loadTemplates = async (applyDefault = false) => {
    try {
      const { data } = await labelsApi.getTemplates();
      setTemplates(data);
      if (applyDefault) {
        const def = data.find(t => t.is_default);
        if (def) applyTemplate(def);
      }
    } catch { console.error('Failed to load templates'); }
  };

  /* ── Template operations ── */

  const applyTemplate = (tpl: LabelTemplate) => {
    setCurrentTplId(tpl.id ?? null);
    setLabelW(tpl.width);
    setLabelH(tpl.height);
    setTemplateName(tpl.name);
    try {
      const parsed: LabelElement[] = JSON.parse(tpl.template_json || '[]');
      setElements(parsed.map((e, i) => ({ ...e, id: `elem-${i}` })));
    } catch {
      setElements([]);
    }
    setSelectedId(null);
  };

  const newTemplate = () => {
    setCurrentTplId(null);
    setTemplateName('Neues Template');
    setLabelW(62);
    setLabelH(29);
    setElements([]);
    setSelectedId(null);
  };

  const saveTemplate = async () => {
    if (!templateName.trim()) return toast('error', 'Bitte Template-Namen eingeben!');
    setSaving(true);
    try {
      const tpl: LabelTemplate = {
        name: templateName,
        description: '',
        width: labelW,
        height: labelH,
        // Strip only runtime-only UI fields; keep image_data so uploaded logos survive save/reload
        template_json: JSON.stringify(
          elements.map(({ id: _id, aspect_ratio_locked: _ar, ...rest }) => rest)
        ),
        is_default: false,
      };
      if (currentTplId) {
        await labelsApi.updateTemplate(currentTplId, tpl);
        toast('success', 'Template aktualisiert');
      } else {
        const { data } = await labelsApi.createTemplate(tpl);
        setCurrentTplId(data.id ?? null);
        toast('success', 'Template erstellt');
      }
      await loadTemplates(false);
    } catch { toast('error', 'Fehler beim Speichern'); }
    finally { setSaving(false); }
  };

  const setAsDefault = async () => {
    if (!currentTplId) return toast('error', 'Bitte Template zuerst speichern!');
    try {
      await labelsApi.updateTemplate(currentTplId, { is_default: true } as Partial<LabelTemplate>);
      await loadTemplates();
      toast('success', 'Als Standard gesetzt ★');
    } catch { toast('error', 'Fehler'); }
  };

  const deleteTemplate = async (id: number) => {
    if (!confirm('Template wirklich löschen?')) return;
    try {
      await labelsApi.deleteTemplate(id);
      await loadTemplates();
      if (currentTplId === id) newTemplate();
      toast('success', 'Template gelöscht');
    } catch { toast('error', 'Fehler beim Löschen'); }
  };

  /* ── Element operations ── */

  const addElement = (type: DesignElement['type']) => {
    const sizes: Record<string, [number, number]> = {
      qrcode:  [Math.min(25, labelH - 2), Math.min(25, labelH - 2)],
      barcode: [Math.min(50, labelW - 4), Math.min(15, labelH - 4)],
      image:   [Math.min(20, labelW - 4), Math.min(20, labelH - 4)],
      text:    [Math.min(30, labelW - 4), Math.min(6,  labelH - 4)],
    };
    const [w, h] = sizes[type];
    const el: DesignElement = {
      id:       `elem-${Date.now()}`,
      type,
      x: 2, y: 2,
      width:    Math.max(2, w),
      height:   Math.max(2, h),
      rotation: 0,
      content:  type === 'image' ? '' : 'device_id',
      style: {
        font_size:   8,
        font_weight: 'normal',
        font_family: 'Arial',
        color:       '#000000',
        alignment:   'left',
        format: type === 'barcode' ? 'code128' : type === 'qrcode' ? 'qr' : undefined,
      },
      aspect_ratio_locked: type === 'qrcode' || type === 'image',
    };
    setElements(prev => [...prev, el]);
    setSelectedId(el.id);
  };

  const deleteElement = (id: string) => {
    setElements(prev => prev.filter(e => e.id !== id));
    if (selectedId === id) setSelectedId(null);
  };

  const updateEl = (id: string, updates: Partial<DesignElement>) =>
    setElements(prev => prev.map(e => e.id === id ? { ...e, ...updates } : e));

  const handleImageUpload = (id: string, file: File) => {
    const reader = new FileReader();
    reader.onload = ev => {
      const b64 = ev.target?.result as string;
      const img = new Image();
      img.onload = () => {
        setElements(prev => prev.map(e => {
          if (e.id !== id) return e;
          const up: Partial<DesignElement> = { image_data: b64, content: file.name };
          if (e.aspect_ratio_locked && img.naturalHeight > 0)
            up.height = e.width / (img.naturalWidth / img.naturalHeight);
          return { ...e, ...up };
        }));
      };
      img.src = b64;
    };
    reader.readAsDataURL(file);
  };

  /* ── Drag / resize ── */

  const onElementMouseDown = (e: React.MouseEvent, elem: DesignElement) => {
    e.preventDefault();
    e.stopPropagation();
    setSelectedId(elem.id);
    dragRef.current = {
      type: 'move',
      elementId: elem.id,
      startX: e.clientX, startY: e.clientY,
      origX: elem.x,     origY: elem.y,
      origW: elem.width, origH: elem.height,
      origRatio: elem.height > 0 ? elem.width / elem.height : 1,
    };
  };

  const onHandleMouseDown = (e: React.MouseEvent, elem: DesignElement, handle: ResizeHandle) => {
    e.preventDefault();
    e.stopPropagation();
    dragRef.current = {
      type: 'resize',
      handle,
      elementId: elem.id,
      startX: e.clientX, startY: e.clientY,
      origX: elem.x,     origY: elem.y,
      origW: elem.width, origH: elem.height,
      origRatio: elem.height > 0 ? elem.width / elem.height : 1,
    };
  };

  // Document-level handlers so drag continues even when mouse leaves the editor div
  const handleDocMouseMove = useCallback((e: MouseEvent) => {
    const ds = dragRef.current;
    if (!ds) return;

    const lw  = labelWRef.current;
    const lh  = labelHRef.current;
    const sc  = Math.min(EDITOR_MAX / lw, EDITOR_MAX / lh);
    const snap = snapRef.current;

    const dx = (e.clientX - ds.startX) / sc;
    const dy = (e.clientY - ds.startY) / sc;

    if (ds.type === 'move') {
      const nx = snapVal(Math.max(0, Math.min(lw - ds.origW, ds.origX + dx)), GRID_MM, snap);
      const ny = snapVal(Math.max(0, Math.min(lh - ds.origH, ds.origY + dy)), GRID_MM, snap);
      setElements(prev => prev.map(el =>
        el.id === ds.elementId ? { ...el, x: nx, y: ny } : el
      ));
      return;
    }

    const h      = ds.handle!;
    const locked = elementsRef.current.find(el => el.id === ds.elementId)?.aspect_ratio_locked ?? false;

    let nx = ds.origX, ny = ds.origY, nw = ds.origW, nh = ds.origH;

    if (h.includes('e')) nw = Math.max(2, ds.origW + dx);
    if (h.includes('s')) nh = Math.max(2, ds.origH + dy);
    if (h.includes('w')) { nw = Math.max(2, ds.origW - dx); nx = ds.origX + ds.origW - nw; }
    if (h.includes('n')) { nh = Math.max(2, ds.origH - dy); ny = ds.origY + ds.origH - nh; }

    if (locked && (h === 'nw' || h === 'ne' || h === 'se' || h === 'sw')) {
      nh = nw / ds.origRatio;
      if (h === 'ne') ny = ds.origY + ds.origH - nh;
      if (h === 'sw') nx = ds.origX + ds.origW - nw;
      if (h === 'nw') { ny = ds.origY + ds.origH - nh; nx = ds.origX + ds.origW - nw; }
    }

    nx = snapVal(Math.max(0, nx), GRID_MM, snap);
    ny = snapVal(Math.max(0, ny), GRID_MM, snap);
    nw = snapVal(Math.max(2, nw), GRID_MM, snap);
    nh = snapVal(Math.max(2, nh), GRID_MM, snap);

    setElements(prev => prev.map(el =>
      el.id === ds.elementId ? { ...el, x: nx, y: ny, width: nw, height: nh } : el
    ));
  }, []);

  const handleDocMouseUp = useCallback(() => { dragRef.current = null; }, []);

  useEffect(() => {
    document.addEventListener('mousemove', handleDocMouseMove);
    document.addEventListener('mouseup',   handleDocMouseUp);
    return () => {
      document.removeEventListener('mousemove', handleDocMouseMove);
      document.removeEventListener('mouseup',   handleDocMouseUp);
    };
  }, [handleDocMouseMove, handleDocMouseUp]);

  /* ── Canvas preview (300 DPI) ── */

  const resolveContent = (field: string, dev: Device): string => {
    const map: Record<string, string | undefined> = {
      device_id:    dev.device_id,
      product_name: dev.product_name,
      serial_number:dev.serial_number,
      barcode:      dev.barcode ?? dev.device_id,
      zone_code:    dev.zone_code,
      zone_name:    dev.zone_name,
      status:       dev.status,
    };
    return map[field] ?? field;
  };

  const drawImg = (
    ctx: CanvasRenderingContext2D,
    src: string, x: number, y: number, w: number, h: number
  ): Promise<void> =>
    new Promise(res => {
      const img = new Image();
      img.onload  = () => { ctx.drawImage(img, x, y, w, h); res(); };
      img.onerror = () => res();
      img.src = src;
    });

  const DEMO_DEVICE: Device = {
    device_id: 'DEMO-001', product_name: 'Demo Produkt',
    serial_number: 'SN-000001', barcode: 'DEMO-001',
    zone_code: 'A1', zone_name: 'Hauptlager', status: 'in_stock',
  };

  const renderPreview = async () => {
    if (!canvasRef.current) return;
    const dev = previewDevice ?? DEMO_DEVICE;
    const canvas = canvasRef.current;
    const ctx    = canvas.getContext('2d');
    if (!ctx) return;

    const dpi = 300;
    const mm  = dpi / 25.4;
    canvas.width  = Math.round(labelW * mm);
    canvas.height = Math.round(labelH * mm);

    ctx.fillStyle = '#ffffff';
    ctx.fillRect(0, 0, canvas.width, canvas.height);
    ctx.strokeStyle = '#cccccc';
    ctx.lineWidth = 1;
    ctx.strokeRect(0.5, 0.5, canvas.width - 1, canvas.height - 1);

    for (const elem of elements) {
      const x = elem.x * mm, y = elem.y * mm;
      const w = elem.width * mm, h = elem.height * mm;
      const content = resolveContent(elem.content, dev);

      try {
        if (elem.type === 'qrcode') {
          const { data } = await labelsApi.generateQRCode(content, Math.floor(w));
          await drawImg(ctx, data.image_data, x, y, w, h);
        } else if (elem.type === 'barcode') {
          const { data } = await labelsApi.generateBarcode(content, Math.floor(w), Math.floor(h));
          await drawImg(ctx, data.image_data, x, y, w, h);
        } else if (elem.type === 'image' && elem.image_data) {
          await drawImg(ctx, elem.image_data, x, y, w, h);
        } else if (elem.type === 'text') {
          ctx.fillStyle = elem.style.color || '#000000';
          const fs = (elem.style.font_size || 8) * (dpi / 96);
          ctx.font      = `${elem.style.font_weight || 'normal'} ${fs}px ${elem.style.font_family || 'Arial'}`;
          ctx.textAlign = (elem.style.alignment as CanvasTextAlign) || 'left';
          ctx.fillText(content, x, y + fs);
        }
      } catch (err) {
        console.error('Render error for element', elem.id, err);
      }
    }
  };

  /* ── Batch label generation ── */

  const generateLabels = async (devs: Device[], cas: CaseSummary[]) => {
    const defaultTpl = templates.find(t => t.is_default);
    if (!defaultTpl) return toast('error', 'Bitte zuerst ein Template als Standard setzen!');

    const origTplId = currentTplId;
    setExporting(true);
    let ok = 0, fail = 0;

    try {
      applyTemplate(defaultTpl);
      await new Promise(r => setTimeout(r, 900));

      for (const dev of devs) {
        setPreviewDevice(dev);
        await new Promise(r => setTimeout(r, 700));
        if (canvasRef.current) {
          try { await labelsApi.saveLabel(dev.device_id, canvasRef.current.toDataURL('image/png')); ok++; }
          catch { fail++; }
        }
      }

      for (const c of cas) {
        const fake: Device = {
          device_id:    `CASE-${c.case_id}`,
          product_name: c.name,
          status:       c.status,
          zone_code:    c.zone_code,
          zone_name:    c.zone_name,
          zone_id:      c.zone_id,
        };
        setPreviewDevice(fake);
        await new Promise(r => setTimeout(r, 700));
        if (canvasRef.current) {
          try { await labelsApi.saveCaseLabel(c.case_id, canvasRef.current.toDataURL('image/png')); ok++; }
          catch { fail++; }
        }
      }

      toast('success', `${ok}/${devs.length + cas.length} Labels generiert${fail ? ` · ${fail} Fehler` : ''}`);
    } catch { toast('error', 'Fehler beim Generieren'); }
    finally {
      setExporting(false);
      if (devices.length > 0) setPreviewDevice(devices[0]);
      if (origTplId) {
        const orig = templates.find(t => t.id === origTplId);
        if (orig) applyTemplate(orig);
      }
    }
  };

  const generateAllLabels = () => generateLabels(devices, cases);

  const generateMissingLabels = async () => {
    const devsNo  = devices.filter(d => !d.label_path);
    const casesNo = cases.filter(c => !c.label_path);
    const total   = devsNo.length + casesNo.length;
    if (total === 0) return toast('success', 'Alle Items haben bereits Labels!');
    if (!confirm(`${total} Items ohne Labels (${devsNo.length} Devices + ${casesNo.length} Cases).\nJetzt generieren?`)) return;
    await generateLabels(devsNo, casesNo);
    await loadDevices();
    await loadCases();
  };

  const exportZip = async () => {
    setExporting(true);
    try {
      const zip = new JSZip();
      let n = 0;
      for (const d of devices.filter(d => d.label_path)) {
        const r = await fetch(d.label_path!);
        if (r.ok) { zip.file(`devices/${d.device_id}.png`, await r.blob()); n++; }
      }
      for (const c of cases.filter(c => c.label_path)) {
        const r = await fetch(c.label_path!);
        if (r.ok) { zip.file(`cases/CASE-${c.case_id}.png`, await r.blob()); n++; }
      }
      if (n === 0) return toast('error', 'Keine generierten Labels vorhanden!');
      const blob = await zip.generateAsync({ type: 'blob' });
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = `labels_${new Date().toISOString().split('T')[0]}.zip`;
      a.click();
      URL.revokeObjectURL(a.href);
      toast('success', `${n} Labels als ZIP exportiert`);
    } catch { toast('error', 'Fehler beim ZIP-Export'); }
    finally { setExporting(false); }
  };

  const printPreview = () => {
    if (!canvasRef.current) return;
    const dataUrl = canvasRef.current.toDataURL('image/png');
    const win = window.open('', '_blank');
    if (!win) return;
    // @page sets physical paper size = label size so printer outputs exact dimensions
    const style = win.document.createElement('style');
    style.textContent = [
      `@page { size: ${labelW}mm ${labelH}mm; margin: 0; }`,
      `body { margin: 0; padding: 0; background: white; }`,
      `img { display: block; width: ${labelW}mm; height: ${labelH}mm; }`,
    ].join(' ');
    win.document.head.appendChild(style);
    const img = win.document.createElement('img');
    img.src = dataUrl;
    img.onload = () => { win.focus(); win.print(); win.close(); };
    win.document.body.style.margin = '0';
    win.document.body.appendChild(img);
  };

  /* ── Derived ── */

  const selectedElem = elements.find(e => e.id === selectedId);
  const canGenerate  = !exporting && (devices.length > 0 || cases.length > 0);
  const currentIsDef = templates.find(t => t.id === currentTplId)?.is_default ?? false;

  /* ── JSX ── */

  return (
    <div className="label-designer">

      {/* Header: title + template selector + save controls */}
      <div className="ld-header">
        <div>
          <h1 className="label-designer-header">🏷️ Label Designer</h1>
          <p className="label-designer-sub">Etikett-Templates für Zebra-Drucker</p>
        </div>
        <div className="ld-header-actions">
          <select
            className="input-select"
            value={currentTplId ?? ''}
            onChange={e => {
              const v = e.target.value;
              if (v === 'new') newTemplate();
              else { const t = templates.find(t => t.id === parseInt(v)); if (t) applyTemplate(t); }
            }}
            style={{ minWidth: 180 }}
          >
            <option value="new">+ Neues Template</option>
            {templates.map(t => (
              <option key={t.id} value={t.id}>{t.name}{t.is_default ? ' ★' : ''}</option>
            ))}
          </select>

          <input
            className="input-field"
            value={templateName}
            onChange={e => setTemplateName(e.target.value)}
            placeholder="Template Name"
            style={{ width: 180 }}
          />

          <button onClick={saveTemplate} disabled={saving} className="btn-save">
            <Save size={15} /> {saving ? 'Speichert…' : currentTplId ? 'Aktualisieren' : 'Speichern'}
          </button>

          {currentTplId && !currentIsDef && (
            <button onClick={setAsDefault} className="btn-add">★ Standard</button>
          )}

          {currentTplId && (
            <button onClick={() => deleteTemplate(currentTplId)} className="btn-delete-small">
              <Trash2 size={15} />
            </button>
          )}
        </div>
      </div>

      {/* Three-column grid */}
      <div className="designer-grid">

        {/* ── Left panel: tools + properties ── */}
        <div className="designer-panel glass-dark">

          <div className="panel-section">
            <h3>Label-Größe</h3>
            <select className="input-select w-full"
              onChange={e => {
                const p = ZEBRA_PRESETS[+e.target.value];
                if (p.w > 0) { setLabelW(p.w); setLabelH(p.h); }
              }}
            >
              {ZEBRA_PRESETS.map((p, i) => <option key={i} value={i}>{p.label}</option>)}
            </select>
            <div className="input-group">
              <input type="number" step="0.1" value={labelW}
                onChange={e => setLabelW(+e.target.value)} className="input-field-small" />
              <span>×</span>
              <input type="number" step="0.1" value={labelH}
                onChange={e => setLabelH(+e.target.value)} className="input-field-small" />
              <span className="ld-unit">mm</span>
            </div>
          </div>

          <div className="panel-section">
            <h3>Elemente hinzufügen</h3>
            <div className="button-group">
              <button onClick={() => addElement('qrcode')}  className="btn-add"><QrCode size={15} /> QR-Code</button>
              <button onClick={() => addElement('barcode')} className="btn-add"><Barcode size={15} /> Barcode</button>
              <button onClick={() => addElement('text')}    className="btn-add"><Type size={15} /> Text</button>
              <button onClick={() => addElement('image')}   className="btn-add"><ImageIcon size={15} /> Bild</button>
            </div>
          </div>

          <div className="panel-section">
            <div className="ld-toggle-row">
              <span>Magnetisches Einrasten ({GRID_MM} mm)</span>
              <button
                className={`ld-toggle-btn${snapEnabled ? ' active' : ''}`}
                onClick={() => setSnapEnabled(v => !v)}
              >
                <Grid3x3 size={13} /> {snapEnabled ? 'Ein' : 'Aus'}
              </button>
            </div>
          </div>

          {/* Selected-element properties */}
          {selectedElem && (
            <div className="panel-section">
              <div className="section-header">
                <h3>Eigenschaften</h3>
                <button onClick={() => deleteElement(selectedElem.id)} className="btn-delete-small">
                  <Trash2 size={14} />
                </button>
              </div>

              <div className="property-grid">
                <label>X (mm)</label>
                <input type="number" step="0.5"
                  value={Math.round(selectedElem.x * 10) / 10}
                  onChange={e => updateEl(selectedElem.id, { x: +e.target.value })}
                  className="input-field-small" />

                <label>Y (mm)</label>
                <input type="number" step="0.5"
                  value={Math.round(selectedElem.y * 10) / 10}
                  onChange={e => updateEl(selectedElem.id, { y: +e.target.value })}
                  className="input-field-small" />

                <label>Breite</label>
                <div className="ld-ratio-row">
                  <input type="number" step="0.5"
                    value={Math.round(selectedElem.width * 10) / 10}
                    onChange={e => {
                      const nw = +e.target.value;
                      if (selectedElem.aspect_ratio_locked && selectedElem.height > 0)
                        updateEl(selectedElem.id, { width: nw, height: nw / (selectedElem.width / selectedElem.height) });
                      else
                        updateEl(selectedElem.id, { width: nw });
                    }}
                    className="input-field-small" />
                  {(selectedElem.type === 'image' || selectedElem.type === 'qrcode') && (
                    <button
                      className={`ld-lock-btn${selectedElem.aspect_ratio_locked ? ' locked' : ''}`}
                      onClick={() => updateEl(selectedElem.id, { aspect_ratio_locked: !selectedElem.aspect_ratio_locked })}
                      title={selectedElem.aspect_ratio_locked ? 'Seitenverhältnis gesperrt' : 'Seitenverhältnis frei'}
                    >
                      {selectedElem.aspect_ratio_locked ? <Lock size={12} /> : <Unlock size={12} />}
                    </button>
                  )}
                </div>

                <label>Höhe</label>
                <input type="number" step="0.5"
                  value={Math.round(selectedElem.height * 10) / 10}
                  onChange={e => {
                    const nh = +e.target.value;
                    if (selectedElem.aspect_ratio_locked && selectedElem.width > 0)
                      updateEl(selectedElem.id, { height: nh, width: nh * (selectedElem.width / selectedElem.height) });
                    else
                      updateEl(selectedElem.id, { height: nh });
                  }}
                  className="input-field-small" />

                {selectedElem.type !== 'image' && (
                  <>
                    <label>Inhalt</label>
                    <select value={selectedElem.content}
                      onChange={e => updateEl(selectedElem.id, { content: e.target.value })}
                      className="input-field-small"
                    >
                      {FIELD_OPTIONS.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
                    </select>
                  </>
                )}

                {selectedElem.type === 'image' && (
                  <>
                    <label>Bild</label>
                    <input type="file" accept="image/*"
                      onChange={e => { const f = e.target.files?.[0]; if (f) handleImageUpload(selectedElem.id, f); }}
                      className="input-field-small"
                      style={{ padding: '0.2rem' }}
                    />
                    {selectedElem.image_data && (
                      <div style={{ gridColumn: '1/-1', fontSize: '0.72rem', color: 'rgba(255,255,255,0.4)' }}>
                        ✓ {selectedElem.content}
                      </div>
                    )}
                  </>
                )}

                {selectedElem.type === 'text' && (
                  <>
                    <label>Schriftgröße</label>
                    <input type="number" value={selectedElem.style.font_size ?? 8}
                      onChange={e => updateEl(selectedElem.id, { style: { ...selectedElem.style, font_size: +e.target.value } })}
                      className="input-field-small" />

                    <label>Schriftart</label>
                    <select value={selectedElem.style.font_family ?? 'Arial'}
                      onChange={e => updateEl(selectedElem.id, { style: { ...selectedElem.style, font_family: e.target.value } })}
                      className="input-field-small"
                    >
                      {['Arial', 'Ubuntu', 'Times New Roman', 'Courier New', 'Verdana'].map(f => (
                        <option key={f} value={f}>{f}</option>
                      ))}
                    </select>

                    <label>Stil</label>
                    <select value={selectedElem.style.font_weight ?? 'normal'}
                      onChange={e => updateEl(selectedElem.id, { style: { ...selectedElem.style, font_weight: e.target.value } })}
                      className="input-field-small"
                    >
                      <option value="normal">Normal</option>
                      <option value="bold">Fett</option>
                    </select>

                    <label>Farbe</label>
                    <input type="color"
                      value={selectedElem.style.color ?? '#000000'}
                      onChange={e => updateEl(selectedElem.id, { style: { ...selectedElem.style, color: e.target.value } })}
                      style={{ width: '100%', height: 32, cursor: 'pointer', border: 'none', borderRadius: 4 }}
                    />
                  </>
                )}
              </div>
            </div>
          )}
        </div>

        {/* ── Center: WYSIWYG editor + canvas preview ── */}
        <div className="designer-canvas-area glass-dark">
          <div className="canvas-header">
            <h3>Editor</h3>
            <select className="input-select-small"
              value={previewDevice?.device_id ?? ''}
              onChange={e => {
                const v = e.target.value;
                if (v.startsWith('CASE-')) {
                  const cid = parseInt(v.replace('CASE-', ''));
                  const c   = cases.find(c => c.case_id === cid);
                  if (c) setPreviewDevice({
                    device_id: `CASE-${c.case_id}`, product_name: c.name,
                    status: c.status, zone_code: c.zone_code, zone_name: c.zone_name, zone_id: c.zone_id,
                  });
                } else {
                  const d = devices.find(d => d.device_id === v);
                  if (d) setPreviewDevice(d);
                }
              }}
            >
              <optgroup label="Devices">
                {devices.map(d => <option key={d.device_id} value={d.device_id}>{d.device_id} – {d.product_name}</option>)}
              </optgroup>
              <optgroup label="Cases">
                {cases.map(c => <option key={c.case_id} value={`CASE-${c.case_id}`}>CASE-{c.case_id} – {c.name}</option>)}
              </optgroup>
            </select>
            <button
              className={`btn-toggle-preview${showPreview ? ' active' : ''}`}
              onClick={() => setShowPreview(p => !p)}
              title={showPreview ? 'Vorschau ausblenden' : 'Vorschau anzeigen'}
            >
              {showPreview ? <EyeOff size={15} /> : <Eye size={15} />}
              <span>{showPreview ? 'Vorschau' : 'Vorschau'}</span>
            </button>
          </div>

          <div className="ld-split">

            {/* Interactive WYSIWYG editor */}
            <div className="ld-col">
              <p className="ld-col-label">
                Design-Editor&nbsp;
                <span className="ld-dims">{labelW} × {labelH} mm · {elements.length} El.</span>
              </p>
              <div className="ld-scroll">
                <div
                  className="ld-label-editor"
                  style={{
                    width:  edW,
                    height: edH,
                    backgroundSize:  snapEnabled ? `${GRID_MM * scale}px ${GRID_MM * scale}px` : undefined,
                    backgroundImage: snapEnabled ? undefined : 'none',
                  }}
                  onClick={e => { if (e.target === e.currentTarget) setSelectedId(null); }}
                >
                  {elements.map(elem => (
                    <div
                      key={elem.id}
                      className={`ld-element${selectedId === elem.id ? ' ld-selected' : ''}`}
                      style={{
                        left:   elem.x * scale,
                        top:    elem.y * scale,
                        width:  elem.width * scale,
                        height: elem.height * scale,
                        cursor: dragRef.current ? 'grabbing' : 'grab',
                      }}
                      onMouseDown={e => onElementMouseDown(e, elem)}
                    >
                      {elem.type === 'image' && elem.image_data && (
                        <img src={elem.image_data} draggable={false}
                          style={{ width: '100%', height: '100%', objectFit: 'contain', display: 'block', pointerEvents: 'none' }} />
                      )}
                      {elem.type === 'image' && !elem.image_data && (
                        <div className="ld-placeholder"><ImageIcon size={Math.min(elem.width * scale * 0.5, 24)} /></div>
                      )}
                      {elem.type === 'qrcode' && (
                        imgCache[elem.id]
                          ? <img src={imgCache[elem.id]} draggable={false}
                              style={{ width: '100%', height: '100%', objectFit: 'contain', display: 'block', pointerEvents: 'none' }} />
                          : <div className="ld-placeholder"><QrCode size={Math.min(elem.width * scale * 0.65, 28)} /></div>
                      )}
                      {elem.type === 'barcode' && (
                        imgCache[elem.id]
                          ? <img src={imgCache[elem.id]} draggable={false}
                              style={{ width: '100%', height: '100%', objectFit: 'contain', display: 'block', pointerEvents: 'none' }} />
                          : <div className="ld-barcode-preview">
                              <div className="ld-barcode-lines" />
                              <span className="ld-barcode-text"
                                style={{ fontSize: Math.min(elem.height * scale * 0.28, 9) }}>
                                {FIELD_OPTIONS.find(o => o.value === elem.content)?.label ?? elem.content}
                              </span>
                            </div>
                      )}
                      {elem.type === 'text' && (
                        <div className="ld-text-preview" style={{
                          fontSize:   Math.min((elem.style.font_size ?? 8) * scale * 0.52, 13),
                          fontWeight: elem.style.font_weight ?? 'normal',
                          fontFamily: elem.style.font_family ?? 'Arial',
                          textAlign:  (elem.style.alignment as React.CSSProperties['textAlign']) ?? 'left',
                        }}>
                          {FIELD_OPTIONS.find(o => o.value === elem.content)?.label ?? elem.content}
                        </div>
                      )}

                      {/* 8-point resize handles – only on selected element */}
                      {selectedId === elem.id && HANDLES.map(h => (
                        <div key={h} className="ld-handle"
                          style={{ position: 'absolute', ...HANDLE_POS[h] }}
                          onMouseDown={e => onHandleMouseDown(e, elem, h)} />
                      ))}
                    </div>
                  ))}
                </div>
              </div>
              <p className="ld-hint">Ziehen = verschieben · Ecken/Kanten ziehen = skalieren</p>
            </div>

            {/* Canvas print preview – always mounted so canvasRef stays valid */}
            <div className="ld-col" style={{ display: showPreview ? undefined : 'none' }}>
              <p className="ld-col-label">Druckvorschau <span className="ld-dims">300 DPI</span></p>
              <div className="ld-scroll">
                <canvas ref={canvasRef} className="ld-preview-canvas" />
              </div>
            </div>

          </div>
        </div>

        {/* ── Right panel: element list + actions ── */}
        <div className="designer-panel glass-dark">

          <div className="panel-section">
            <h3>Elemente ({elements.length})</h3>
            <div className="elements-list">
              {elements.map(elem => (
                <div
                  key={elem.id}
                  className={`element-item${selectedId === elem.id ? ' active' : ''}`}
                  onClick={() => setSelectedId(elem.id)}
                >
                  <div className="element-icon">
                    {elem.type === 'qrcode'  && <QrCode size={14} />}
                    {elem.type === 'barcode' && <Barcode size={14} />}
                    {elem.type === 'text'    && <Type size={14} />}
                    {elem.type === 'image'   && <ImageIcon size={14} />}
                  </div>
                  <div className="element-info">
                    <div className="element-type">
                      {elem.type === 'qrcode' ? 'QR-Code'
                        : elem.type === 'barcode' ? 'Barcode'
                        : elem.type === 'image'   ? 'Bild'
                        : 'Text'}
                    </div>
                    <div className="element-content">
                      {Math.round(elem.x * 10) / 10},{Math.round(elem.y * 10) / 10} ·&nbsp;
                      {Math.round(elem.width * 10) / 10}×{Math.round(elem.height * 10) / 10} mm
                    </div>
                  </div>
                  <button
                    onClick={e => { e.stopPropagation(); deleteElement(elem.id); }}
                    className="btn-delete-mini"
                  >
                    <Trash2 size={12} />
                  </button>
                </div>
              ))}
              {elements.length === 0 && <div className="empty-state">Keine Elemente</div>}
            </div>
          </div>

          <div className="panel-section">
            <h3>Aktionen</h3>
            <div className="action-buttons">
              <button onClick={printPreview} disabled={!previewDevice} className="btn-action">
                <Printer size={15} /> Drucken
              </button>
              <button onClick={generateMissingLabels} disabled={!canGenerate}
                className="btn-action" style={{ background: '#10b981' }}>
                <Save size={15} /> {exporting ? 'Generiere…' : 'Fehlende Labels'}
              </button>
              <button onClick={generateAllLabels} disabled={!canGenerate} className="btn-action btn-primary">
                <Save size={15} /> {exporting
                  ? `Generiere ${devices.length + cases.length}…`
                  : `Alle (${devices.length + cases.length})`}
              </button>
              <button onClick={exportZip} disabled={!canGenerate} className="btn-action">
                <Download size={15} /> ZIP Export
              </button>
            </div>
          </div>

        </div>
      </div>
    </div>
  );
}
