import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Barcode, Boxes, BriefcaseBusiness, Check, Copy, Download, Image as ImageIcon,
  MapPin, Maximize2, Minus, Plus, Printer, QrCode, RefreshCw, Save, Search, Settings2,
  Tags, Trash2, Type, Usb, X,
} from 'lucide-react';
import {
  labelsApi,
  type LabelElement,
  type LabelFieldDefinition,
  type LabelPrinter,
  type LabelPrintJob,
  type LabelTarget,
  type LabelTargetType,
  type LabelTemplate,
} from '../lib/api';
import { toast } from '../lib/toast';
import './LabelDesignerPage.css';

type StudioTab = 'designer' | 'print' | 'printers';
type DragMode = 'move' | 'resize';
type ResizeHandle = 'nw' | 'n' | 'ne' | 'e' | 'se' | 's' | 'sw' | 'w';
type DesignElement = LabelElement & { id: string };

const RESIZE_HANDLES: ResizeHandle[] = ['nw', 'n', 'ne', 'e', 'se', 's', 'sw', 'w'];

const TARGET_TYPES: Array<{ type: LabelTargetType; label: string; icon: typeof Boxes }> = [
  { type: 'device', label: 'Geräte', icon: Tags },
  { type: 'product', label: 'Produkte & Kabel', icon: Boxes },
  { type: 'case', label: 'Cases', icon: BriefcaseBusiness },
  { type: 'zone', label: 'Lagerzonen', icon: MapPin },
];

const LABEL_PRESETS = [
  { label: '51 × 25 mm', width: 51, height: 25 },
  { label: '62 × 29 mm', width: 62, height: 29 },
  { label: '57 × 32 mm', width: 57.2, height: 31.8 },
  { label: '101 × 51 mm', width: 101.6, height: 50.8 },
  { label: '101 × 25 mm', width: 101.6, height: 25.4 },
  { label: '38 × 25 mm', width: 38.1, height: 25.4 },
];

const EMPTY_PRINTER: LabelPrinter = {
  name: '', driver: 'zpl_tcp', address: '', port: 9100, dpi: 203,
  is_default: false, is_active: true,
};

const sampleFields: Record<LabelTargetType, Record<string, string>> = {
  device: { code: 'DEV-0001', name: 'Demo Gerät', device_id: 'DEV-0001', product_name: 'Demo Gerät', serial_number: 'SN-10001', barcode: 'DEV-0001', status: 'in_storage', zone_code: 'A-01', zone_name: 'Hauptlager', category: 'Audio' },
  product: { code: 'PROD-000001', name: 'XLR Kabel 10 m', product_id: '1', product_name: 'XLR Kabel 10 m', generic_barcode: 'PROD-000001', barcode: 'PROD-000001', stock_quantity: '12', unit: 'Stk', category: 'Kabel', description: '' },
  case: { code: 'CASE-1', name: 'Audio Case', case_id: 'CASE-1', barcode: 'CASE-1', status: 'free', zone_code: 'C-01', zone_name: 'Case-Lager', dimensions: '60 × 40 × 40', weight: '18 kg', description: '' },
  zone: { code: 'ZONE-0001', name: 'Regal A / Fach 1', zone_id: '1', zone_code: 'A-01', barcode: 'ZONE-0001', type: 'shelf', location: 'Hauptlager', capacity: '20', description: '' },
};

function uid() {
  return `label-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

function withIDs(elements: LabelElement[]): DesignElement[] {
  return elements.map(element => ({ ...element, id: uid() }));
}

function cleanElements(elements: DesignElement[]): LabelElement[] {
  return elements.map(({ id, ...element }) => {
    void id;
    return element;
  });
}

function defaultElement(type: LabelElement['type'], field = 'code'): DesignElement {
  const base = {
    id: uid(), type, x: 2, y: 2, width: 24, height: type === 'text' ? 6 : 18,
    rotation: 0, content: field,
    style: { font_size: 9, font_weight: type === 'text' ? 'bold' : 'normal', font_family: 'Arial', color: '#000000', alignment: 'left', format: type === 'qrcode' ? 'qr' : 'code128' },
  } satisfies DesignElement;
  if (type === 'image') return { ...base, content: 'logo', image_data: '' };
  return base;
}

function statusBadge(status: LabelPrintJob['status']) {
  if (status === 'completed') return 'badge-success';
  if (status === 'failed') return 'badge-danger';
  if (status === 'printing') return 'badge-info';
  return 'badge-neutral';
}

function targetLabel(type: LabelTargetType) {
  return TARGET_TYPES.find(item => item.type === type)?.label ?? type;
}

function openBrowserPrint(images: string[], width: number, height: number, copies: number, win: Window) {
  win.document.title = 'WarehouseCore Labels';
  const style = win.document.createElement('style');
  style.textContent = `@page { size: ${width}mm ${height}mm; margin: 0; } body { margin: 0; background: white; } img { display:block; width:${width}mm; height:${height}mm; break-after:page; } img:last-child { break-after:auto; }`;
  win.document.head.appendChild(style);
  for (const src of images) {
    for (let copy = 0; copy < copies; copy += 1) {
      const image = win.document.createElement('img');
      image.src = src;
      win.document.body.appendChild(image);
    }
  }
  const allImages = Array.from(win.document.images);
  Promise.all(allImages.map(image => image.complete ? Promise.resolve() : new Promise<void>(resolve => {
    image.onload = () => resolve();
    image.onerror = () => resolve();
  }))).then(() => {
    win.focus();
    win.print();
  });
}

export default function LabelDesignerPage() {
  const [tab, setTab] = useState<StudioTab>('designer');
  const [targetType, setTargetType] = useState<LabelTargetType>('device');
  const [templates, setTemplates] = useState<LabelTemplate[]>([]);
  const [activeTemplateID, setActiveTemplateID] = useState<number | null>(null);
  const [templateName, setTemplateName] = useState('Neues Template');
  const [isDefault, setIsDefault] = useState(false);
  const [labelWidth, setLabelWidth] = useState(51);
  const [labelHeight, setLabelHeight] = useState(25);
  const [elements, setElements] = useState<DesignElement[]>([]);
  const [selectedElementID, setSelectedElementID] = useState<string | null>(null);
  const [fields, setFields] = useState<LabelFieldDefinition[]>([]);
  const [targets, setTargets] = useState<LabelTarget[]>([]);
  const [previewTarget, setPreviewTarget] = useState<LabelTarget | null>(null);
  const [selectedTargets, setSelectedTargets] = useState<Set<string>>(new Set());
  const [search, setSearch] = useState('');
  const [printers, setPrinters] = useState<LabelPrinter[]>([]);
  const [jobs, setJobs] = useState<LabelPrintJob[]>([]);
  const [printerForm, setPrinterForm] = useState<LabelPrinter>(EMPTY_PRINTER);
  const [selectedPrinterID, setSelectedPrinterID] = useState(0);
  const [copies, setCopies] = useState(1);
  const [busy, setBusy] = useState(false);
  const [codeImages, setCodeImages] = useState<Record<string, string>>({});
  const [canvasZoom, setCanvasZoom] = useState(100);
  const [stageSize, setStageSize] = useState({ width: 620, height: 540 });
  const canvasRef = useRef<HTMLDivElement>(null);
  const stageRef = useRef<HTMLDivElement>(null);

  const activeTemplate = useMemo(
    () => templates.find(template => template.id === activeTemplateID) ?? null,
    [activeTemplateID, templates],
  );
  const typeTemplates = useMemo(
    () => templates.filter(template => template.target_type === targetType),
    [targetType, templates],
  );
  const selectedElement = elements.find(element => element.id === selectedElementID) ?? null;
  const previewFields = previewTarget?.fields ?? sampleFields[targetType];
  const fitScale = Math.max(1, Math.min((stageSize.width - 48) / labelWidth, (stageSize.height - 48) / labelHeight, 12));
  const scale = fitScale * canvasZoom / 100;

  const loadTemplates = useCallback(async () => {
    const { data } = await labelsApi.getTemplates();
    const nextTemplates = data ?? [];
    setTemplates(nextTemplates);
    return nextTemplates;
  }, []);

  const loadPrinters = useCallback(async () => {
    const { data } = await labelsApi.getPrinters();
    const nextPrinters = data ?? [];
    setPrinters(nextPrinters);
    const defaultPrinter = nextPrinters.find(printer => printer.is_default && printer.is_active) ?? nextPrinters.find(printer => printer.is_active);
    if (defaultPrinter?.id) setSelectedPrinterID(current => current || defaultPrinter.id!);
  }, []);

  const loadJobs = useCallback(async () => {
    const { data } = await labelsApi.getPrintJobs(100);
    setJobs(data ?? []);
  }, []);

  const loadTargets = useCallback(async (type: LabelTargetType, term: string) => {
    const { data } = await labelsApi.getTargets(type, term, 500);
    const nextTargets = data ?? [];
    setTargets(nextTargets);
    setSelectedTargets(new Set());
    if (nextTargets[0]) {
      const detail = await labelsApi.getTarget(type, nextTargets[0].id);
      setPreviewTarget(detail.data);
    } else {
      setPreviewTarget(null);
    }
  }, []);

  useEffect(() => {
    Promise.all([loadTemplates(), loadPrinters(), loadJobs()]).catch(error => toast.error(String(error)));
  }, [loadJobs, loadPrinters, loadTemplates]);

  useEffect(() => {
    labelsApi.getFields(targetType).then(response => setFields(response.data)).catch(error => toast.error(String(error)));
    const timer = window.setTimeout(() => loadTargets(targetType, search).catch(error => toast.error(String(error))), 250);
    return () => window.clearTimeout(timer);
  }, [loadTargets, search, targetType]);

  useEffect(() => {
    const available = templates.filter(template => template.target_type === targetType);
    const next = available.find(template => template.is_default) ?? available[0];
    if (next) {
      applyTemplate(next);
    } else {
      startNewTemplate();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [targetType, templates.length]);

  useEffect(() => {
    let cancelled = false;
    for (const element of elements) {
      if (element.type !== 'barcode' && element.type !== 'qrcode') continue;
      const content = previewFields[element.content] ?? element.content;
      const key = `${element.type}|${content}|${element.width}|${element.height}`;
      if (codeImages[key]) continue;
      const request = element.type === 'qrcode'
        ? labelsApi.generateQRCode(content, Math.max(100, Math.round(element.width * 12)))
        : labelsApi.generateBarcode(content, Math.max(123, Math.round(element.width * 12)), Math.max(50, Math.round(element.height * 12)));
      request.then(response => {
        if (!cancelled) setCodeImages(current => ({ ...current, [key]: response.data.image_data }));
      }).catch(() => undefined);
    }
    return () => { cancelled = true; };
  }, [codeImages, elements, previewFields]);

  useEffect(() => {
    const stage = stageRef.current;
    if (!stage) return;
    const updateSize = () => setStageSize({ width: stage.clientWidth, height: stage.clientHeight });
    updateSize();
    const observer = new ResizeObserver(updateSize);
    observer.observe(stage);
    return () => observer.disconnect();
  }, []);

  function applyTemplate(template: LabelTemplate) {
    let parsed: LabelElement[] = [];
    try { parsed = JSON.parse(template.template_json || '[]') as LabelElement[]; } catch { parsed = []; }
    setActiveTemplateID(template.id ?? null);
    setTemplateName(template.name);
    setIsDefault(template.is_default);
    setLabelWidth(Number(template.width));
    setLabelHeight(Number(template.height));
    setElements(withIDs(parsed));
    setSelectedElementID(null);
  }

  function startNewTemplate() {
    setActiveTemplateID(null);
    setTemplateName(`Neues ${targetLabel(targetType)}-Template`);
    setIsDefault(typeTemplates.length === 0);
    setLabelWidth(51);
    setLabelHeight(25);
    setElements([defaultElement('barcode'), { ...defaultElement('text', 'name'), x: 2, y: 16, width: 47 }]);
    setSelectedElementID(null);
  }

  function duplicateTemplate() {
    setActiveTemplateID(null);
    setTemplateName(`${templateName} Kopie`);
    setIsDefault(false);
    setElements(current => withIDs(cleanElements(current)));
  }

  async function saveTemplate() {
    if (!templateName.trim() || labelWidth <= 0 || labelHeight <= 0 || elements.length === 0) {
      toast.error('Name, Größe und mindestens ein Element sind erforderlich.');
      return;
    }
    setBusy(true);
    const payload: LabelTemplate = {
      name: templateName.trim(), description: `${targetLabel(targetType)}-Label`, width: labelWidth,
      height: labelHeight, template_json: JSON.stringify(cleanElements(elements)), is_default: isDefault,
      target_type: targetType, revision: activeTemplate?.revision ?? 1,
    };
    try {
      if (activeTemplateID) await labelsApi.updateTemplate(activeTemplateID, payload);
      else await labelsApi.createTemplate(payload);
      const refreshed = await loadTemplates();
      const saved = refreshed.find(template => template.name === payload.name && template.target_type === targetType);
      if (saved) applyTemplate(saved);
      toast.success('Template gespeichert.');
    } catch (error) { toast.error(`Template konnte nicht gespeichert werden: ${String(error)}`); }
    finally { setBusy(false); }
  }

  async function deleteTemplate() {
    if (!activeTemplateID || !confirm(`Template „${templateName}“ löschen?`)) return;
    try {
      await labelsApi.deleteTemplate(activeTemplateID);
      await loadTemplates();
      toast.success('Template gelöscht.');
    } catch (error) { toast.error(String(error)); }
  }

  function addElement(type: LabelElement['type']) {
    const element = defaultElement(type, fields[0]?.key ?? 'code');
    setElements(current => [...current, element]);
    setSelectedElementID(element.id);
  }

  function updateElement(id: string, updates: Partial<DesignElement>) {
    setElements(current => current.map(element => element.id === id ? { ...element, ...updates } : element));
  }

  function beginDrag(event: React.PointerEvent, element: DesignElement, mode: DragMode, handle: ResizeHandle = 'se') {
    event.preventDefault();
    event.stopPropagation();
    setSelectedElementID(element.id);
    const startX = event.clientX;
    const startY = event.clientY;
    const origin = { x: element.x, y: element.y, width: element.width, height: element.height };
    const onMove = (move: PointerEvent) => {
      const dx = (move.clientX - startX) / scale;
      const dy = (move.clientY - startY) / scale;
      if (mode === 'move') {
        updateElement(element.id, {
          x: Math.max(0, Math.min(labelWidth - origin.width, Math.round((origin.x + dx) * 2) / 2)),
          y: Math.max(0, Math.min(labelHeight - origin.height, Math.round((origin.y + dy) * 2) / 2)),
        });
      } else {
        const minSize = 2;
        let nextX = origin.x;
        let nextY = origin.y;
        let nextWidth = origin.width;
        let nextHeight = origin.height;

        if (handle.includes('e')) {
          nextWidth = Math.max(minSize, Math.min(labelWidth - origin.x, origin.width + dx));
        }
        if (handle.includes('s')) {
          nextHeight = Math.max(minSize, Math.min(labelHeight - origin.y, origin.height + dy));
        }
        if (handle.includes('w')) {
          nextX = Math.max(0, Math.min(origin.x + origin.width - minSize, origin.x + dx));
          nextWidth = origin.width + origin.x - nextX;
        }
        if (handle.includes('n')) {
          nextY = Math.max(0, Math.min(origin.y + origin.height - minSize, origin.y + dy));
          nextHeight = origin.height + origin.y - nextY;
        }

        updateElement(element.id, {
          x: Math.round(nextX * 2) / 2,
          y: Math.round(nextY * 2) / 2,
          width: Math.round(nextWidth * 2) / 2,
          height: Math.round(nextHeight * 2) / 2,
        });
      }
    };
    const onUp = () => {
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
    };
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp, { once: true });
  }

  async function selectPreviewTarget(target: LabelTarget) {
    try {
      const { data } = await labelsApi.getTarget(targetType, target.id);
      setPreviewTarget(data);
    } catch (error) { toast.error(String(error)); }
  }

  function toggleTarget(id: string) {
    setSelectedTargets(current => {
      const next = new Set(current);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  }

  async function generateSelected() {
    if (!activeTemplateID || selectedTargets.size === 0) return;
    setBusy(true);
    let completed = 0;
    try {
      for (const targetID of selectedTargets) {
        await labelsApi.renderTarget({ target_type: targetType, target_id: targetID, template_id: activeTemplateID, save: true });
        completed += 1;
      }
      await loadTargets(targetType, search);
      toast.success(`${completed} Label${completed === 1 ? '' : 's'} erzeugt.`);
    } catch (error) { toast.error(`Nach ${completed} Labels abgebrochen: ${String(error)}`); }
    finally { setBusy(false); }
  }

  async function browserPrint() {
    if (!activeTemplateID || selectedTargets.size === 0) return;
    const printWindow = window.open('', '_blank');
    if (!printWindow) {
      toast.error('Der Browser hat das Druckfenster blockiert.');
      return;
    }
    printWindow.document.body.textContent = 'Labels werden vorbereitet …';
    setBusy(true);
    try {
      const images: string[] = [];
      for (const targetID of selectedTargets) {
        const { data } = await labelsApi.renderTarget({ target_type: targetType, target_id: targetID, template_id: activeTemplateID, save: true });
        images.push(data.image_data);
      }
      printWindow.document.body.replaceChildren();
      openBrowserPrint(images, labelWidth, labelHeight, copies, printWindow);
      await loadTargets(targetType, search);
    } catch (error) {
      printWindow.close();
      toast.error(String(error));
    } finally { setBusy(false); }
  }

  async function directPrint() {
    if (!activeTemplateID || !selectedPrinterID || selectedTargets.size === 0) return;
    setBusy(true);
    try {
      const { data } = await labelsApi.printDirect({
        target_type: targetType, target_ids: Array.from(selectedTargets), template_id: activeTemplateID,
        printer_id: selectedPrinterID, copies,
      });
      const failed = data.jobs.filter(job => job.status === 'failed');
      if (failed.length) toast.error(`${failed.length} Druckauftrag${failed.length === 1 ? '' : 'e'} fehlgeschlagen.`);
      else toast.success(`${data.jobs.length} Druckauftrag${data.jobs.length === 1 ? '' : 'e'} abgeschlossen.`);
      await Promise.all([loadJobs(), loadTargets(targetType, search)]);
    } catch (error) { toast.error(String(error)); }
    finally { setBusy(false); }
  }

  async function savePrinter() {
    setBusy(true);
    try {
      if (printerForm.id) await labelsApi.updatePrinter(printerForm.id, printerForm);
      else await labelsApi.createPrinter(printerForm);
      setPrinterForm(EMPTY_PRINTER);
      await loadPrinters();
      toast.success('Druckerprofil gespeichert.');
    } catch (error) { toast.error(String(error)); }
    finally { setBusy(false); }
  }

  async function deletePrinter(printer: LabelPrinter) {
    if (!printer.id || !confirm(`Drucker „${printer.name}“ löschen?`)) return;
    try {
      await labelsApi.deletePrinter(printer.id);
      await loadPrinters();
      toast.success('Druckerprofil gelöscht.');
    } catch (error) { toast.error(String(error)); }
  }

  function uploadImage(file?: File) {
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      const element = { ...defaultElement('image', 'logo'), image_data: String(reader.result) };
      setElements(current => [...current, element]);
      setSelectedElementID(element.id);
    };
    reader.readAsDataURL(file);
  }

  return (
    <div className="label-studio">
      <header className="ls-header">
        <div>
          <p className="ls-eyebrow">WarehouseCore</p>
          <h1>Label Studio</h1>
          <p>Labels gestalten, erzeugen und direkt an Netzwerkdrucker senden.</p>
        </div>
        <div className="ls-tabs" role="tablist">
          <button className={tab === 'designer' ? 'active' : ''} onClick={() => setTab('designer')}><Settings2 size={16} /> Designer</button>
          <button className={tab === 'print' ? 'active' : ''} onClick={() => setTab('print')}><Printer size={16} /> Druckcenter</button>
          <button className={tab === 'printers' ? 'active' : ''} onClick={() => setTab('printers')}><Usb size={16} /> Drucker</button>
        </div>
      </header>

      <div className="ls-target-tabs">
        {TARGET_TYPES.map(item => {
          const Icon = item.icon;
          return <button key={item.type} className={targetType === item.type ? 'active' : ''} onClick={() => setTargetType(item.type)}><Icon size={16} /> {item.label}</button>;
        })}
      </div>

      {tab === 'designer' && (
        <div className="ls-designer-grid">
          <aside className="card ls-panel">
            <section>
              <div className="ls-section-title"><span>Template</span><button className="ls-icon-button" onClick={startNewTemplate} title="Neues Template"><Plus size={16} /></button></div>
              <select className="input" value={activeTemplateID ?? ''} onChange={event => {
                const template = templates.find(item => item.id === Number(event.target.value));
                if (template) applyTemplate(template);
              }}>
                {typeTemplates.length === 0 && <option value="">Kein Template vorhanden</option>}
                {typeTemplates.map(template => <option key={template.id} value={template.id}>{template.name}{template.is_default ? ' · Standard' : ''}</option>)}
              </select>
              <input className="input" value={templateName} onChange={event => setTemplateName(event.target.value)} placeholder="Template-Name" />
              <label className="ls-check"><input type="checkbox" checked={isDefault} onChange={event => setIsDefault(event.target.checked)} /> Standard für {targetLabel(targetType)}</label>
              <div className="ls-row">
                <button className="btn-action" onClick={duplicateTemplate} disabled={!activeTemplateID}><Copy size={15} /> Duplizieren</button>
                <button className="ls-icon-button danger" onClick={deleteTemplate} disabled={!activeTemplateID}><Trash2 size={16} /></button>
              </div>
            </section>

            <section>
              <div className="ls-section-title"><span>Labelgröße</span></div>
              <select className="input" value={`${labelWidth}x${labelHeight}`} onChange={event => {
                const preset = LABEL_PRESETS.find(item => `${item.width}x${item.height}` === event.target.value);
                if (preset) { setLabelWidth(preset.width); setLabelHeight(preset.height); }
              }}>
                {!LABEL_PRESETS.some(item => item.width === labelWidth && item.height === labelHeight) && <option value={`${labelWidth}x${labelHeight}`}>Benutzerdefiniert</option>}
                {LABEL_PRESETS.map(preset => <option key={preset.label} value={`${preset.width}x${preset.height}`}>{preset.label}</option>)}
              </select>
              <div className="ls-dimensions">
                <label>Breite<input className="input" type="number" min="10" step="0.1" value={labelWidth} onChange={event => setLabelWidth(Number(event.target.value))} /></label>
                <label>Höhe<input className="input" type="number" min="10" step="0.1" value={labelHeight} onChange={event => setLabelHeight(Number(event.target.value))} /></label>
              </div>
            </section>

            <section>
              <div className="ls-section-title"><span>Element hinzufügen</span></div>
              <div className="ls-add-grid">
                <button onClick={() => addElement('text')}><Type size={17} /> Text</button>
                <button onClick={() => addElement('barcode')}><Barcode size={17} /> Barcode</button>
                <button onClick={() => addElement('qrcode')}><QrCode size={17} /> QR-Code</button>
                <label><ImageIcon size={17} /> Bild<input type="file" accept="image/*" onChange={event => uploadImage(event.target.files?.[0])} /></label>
              </div>
            </section>

            <button className="btn-primary ls-full-button" onClick={saveTemplate} disabled={busy}><Save size={16} /> Template speichern</button>
          </aside>

          <main className="card ls-canvas-panel">
            <div className="ls-canvas-toolbar">
              <div>
                <strong>{labelWidth} × {labelHeight} mm</strong>
                <span>300-DPI-Vorschau</span>
              </div>
              <div className="ls-canvas-actions">
                <select className="input" value={previewTarget?.id ?? ''} onChange={event => {
                  const target = targets.find(item => item.id === event.target.value);
                  if (target) selectPreviewTarget(target);
                }}>
                  {targets.map(target => <option key={target.id} value={target.id}>{target.code} · {target.name}</option>)}
                </select>
                <div className="ls-zoom-controls" aria-label="Vorschau-Zoom">
                  <button type="button" onClick={() => setCanvasZoom(current => Math.max(25, current - 25))} aria-label="Verkleinern"><Minus size={15} /></button>
                  <input type="range" min="25" max="200" step="25" value={canvasZoom} onChange={event => setCanvasZoom(Number(event.target.value))} aria-label="Zoomstufe" />
                  <span>{canvasZoom}%</span>
                  <button type="button" onClick={() => setCanvasZoom(current => Math.min(200, current + 25))} aria-label="Vergrößern"><Plus size={15} /></button>
                  <button type="button" className="ls-fit-button" onClick={() => setCanvasZoom(100)} title="Label in Arbeitsfläche einpassen"><Maximize2 size={15} /> Einpassen</button>
                </div>
              </div>
            </div>
            <div ref={stageRef} className="ls-canvas-stage" onPointerDown={() => setSelectedElementID(null)}>
              <div ref={canvasRef} className="ls-label-canvas" style={{ width: labelWidth * scale, height: labelHeight * scale }}>
                {elements.map(element => {
                  const content = previewFields[element.content] ?? element.content;
                  const imageKey = `${element.type}|${content}|${element.width}|${element.height}`;
                  const isSelected = selectedElementID === element.id;
                  return (
                    <div
                      key={element.id}
                      className={`ls-design-element ${isSelected ? 'selected' : ''}`}
                      style={{
                        left: element.x * scale, top: element.y * scale, width: element.width * scale,
                        height: element.height * scale, transform: `rotate(${element.rotation || 0}deg)`,
                        color: element.style.color || 'var(--color-dark)', textAlign: (element.style.alignment as 'left' | 'center' | 'right') || 'left',
                        fontFamily: element.style.font_family || 'Arial', fontWeight: element.style.font_weight || 'normal',
                        fontSize: Math.max(7, (element.style.font_size || 9) * scale / 3),
                      }}
                      onPointerDown={event => beginDrag(event, element, 'move')}
                    >
                      {element.type === 'text' && <span>{content}</span>}
                      {(element.type === 'barcode' || element.type === 'qrcode') && codeImages[imageKey] && <img src={codeImages[imageKey]} alt={content} draggable={false} />}
                      {element.type === 'image' && element.image_data && <img src={element.image_data} alt="Labelgrafik" draggable={false} />}
                      {isSelected && RESIZE_HANDLES.map(handle => (
                        <button
                          key={handle}
                          className={`ls-resize-handle handle-${handle}`}
                          onPointerDown={event => beginDrag(event, element, 'resize', handle)}
                          aria-label={`Größe über ${handle} ändern`}
                        />
                      ))}
                    </div>
                  );
                })}
              </div>
            </div>
          </main>

          <aside className="card ls-panel">
            <section>
              <div className="ls-section-title"><span>Eigenschaften</span>{selectedElement && <button className="ls-icon-button danger" onClick={() => { setElements(current => current.filter(item => item.id !== selectedElement.id)); setSelectedElementID(null); }}><Trash2 size={15} /></button>}</div>
              {!selectedElement && <p className="ls-empty-copy">Element auf dem Label auswählen.</p>}
              {selectedElement && (
                <div className="ls-properties">
                  <label>Inhalt<select className="input" value={fields.some(field => field.key === selectedElement.content) ? selectedElement.content : '__static'} onChange={event => updateElement(selectedElement.id, { content: event.target.value === '__static' ? 'Freier Text' : event.target.value })}>
                    {fields.map(field => <option key={field.key} value={field.key}>{field.label}</option>)}
                    <option value="__static">Freier Text</option>
                  </select></label>
                  {!fields.some(field => field.key === selectedElement.content) && <label>Text<input className="input" value={selectedElement.content} onChange={event => updateElement(selectedElement.id, { content: event.target.value })} /></label>}
                  <div className="ls-dimensions">
                    {(['x', 'y', 'width', 'height'] as const).map(key => <label key={key}>{key.toUpperCase()}<input className="input" type="number" min="0" step="0.5" value={selectedElement[key]} onChange={event => updateElement(selectedElement.id, { [key]: Number(event.target.value) })} /></label>)}
                  </div>
                  <label>Drehung<input className="input" type="number" step="1" value={selectedElement.rotation || 0} onChange={event => updateElement(selectedElement.id, { rotation: Number(event.target.value) })} /></label>
                  {selectedElement.type === 'text' && <>
                    <label>Schriftgröße<input className="input" type="number" min="4" max="72" value={selectedElement.style.font_size ?? 9} onChange={event => updateElement(selectedElement.id, { style: { ...selectedElement.style, font_size: Number(event.target.value) } })} /></label>
                    <label>Ausrichtung<select className="input" value={selectedElement.style.alignment ?? 'left'} onChange={event => updateElement(selectedElement.id, { style: { ...selectedElement.style, alignment: event.target.value } })}><option value="left">Links</option><option value="center">Zentriert</option><option value="right">Rechts</option></select></label>
                    <label className="ls-check"><input type="checkbox" checked={selectedElement.style.font_weight === 'bold'} onChange={event => updateElement(selectedElement.id, { style: { ...selectedElement.style, font_weight: event.target.checked ? 'bold' : 'normal' } })} /> Fett</label>
                  </>}
                </div>
              )}
            </section>
            <section>
              <div className="ls-section-title"><span>Elemente</span><span>{elements.length}</span></div>
              <div className="ls-element-list">
                {elements.map((element, index) => <button key={element.id} className={selectedElementID === element.id ? 'active' : ''} onClick={() => setSelectedElementID(element.id)}><span>{index + 1}</span>{element.type} · {fields.find(field => field.key === element.content)?.label ?? element.content}</button>)}
              </div>
            </section>
          </aside>
        </div>
      )}

      {tab === 'print' && (
        <div className="ls-print-grid">
          <section className="card ls-print-targets">
            <div className="ls-print-toolbar">
              <label className="ls-search"><Search size={16} /><input value={search} onChange={event => setSearch(event.target.value)} placeholder={`${targetLabel(targetType)} suchen`} /></label>
              <button className="btn-action" onClick={() => setSelectedTargets(new Set(targets.map(target => target.id)))}><Check size={15} /> Alle</button>
              <button className="btn-action" onClick={() => setSelectedTargets(new Set())}><X size={15} /> Keine</button>
            </div>
            <div className="ls-target-list">
              {targets.map(target => (
                <label key={target.id} className={selectedTargets.has(target.id) ? 'selected' : ''}>
                  <input type="checkbox" checked={selectedTargets.has(target.id)} onChange={() => toggleTarget(target.id)} />
                  <span className="ls-target-code">{target.code}</span>
                  <span className="ls-target-name"><strong>{target.name}</strong><small>{target.subtitle || target.id}</small></span>
                  {!target.label_path && <span className="badge-neutral">Neu</span>}
                  {target.label_path && target.is_stale && <span className="badge-warning">Veraltet</span>}
                  {target.label_path && !target.is_stale && <span className="badge-success">Aktuell</span>}
                </label>
              ))}
              {targets.length === 0 && <p className="ls-empty-copy">Keine Einträge gefunden.</p>}
            </div>
          </section>

          <aside className="card ls-print-actions">
            <h2>Druckauftrag</h2>
            <label>Template<select className="input" value={activeTemplateID ?? ''} onChange={event => {
              const template = templates.find(item => item.id === Number(event.target.value));
              if (template) applyTemplate(template);
            }}>{typeTemplates.map(template => <option key={template.id} value={template.id}>{template.name}</option>)}</select></label>
            <label>Kopien je Label<input className="input" type="number" min="1" max="1000" value={copies} onChange={event => setCopies(Math.max(1, Number(event.target.value)))} /></label>
            <div className="ls-selection-summary"><strong>{selectedTargets.size}</strong><span>ausgewählt</span><strong>{selectedTargets.size * copies}</strong><span>Labels gesamt</span></div>
            <button className="btn-action ls-full-button" onClick={generateSelected} disabled={busy || selectedTargets.size === 0 || !activeTemplateID}><Download size={16} /> Nur erzeugen</button>
            <button className="btn-primary ls-full-button" onClick={browserPrint} disabled={busy || selectedTargets.size === 0 || !activeTemplateID}><Printer size={16} /> Browserdruck</button>
            <div className="ls-divider"><span>Direktdruck</span></div>
            <select className="input" value={selectedPrinterID} onChange={event => setSelectedPrinterID(Number(event.target.value))}>
              <option value={0}>Drucker auswählen</option>
              {printers.filter(printer => printer.is_active).map(printer => <option key={printer.id} value={printer.id}>{printer.name} · {printer.dpi} DPI</option>)}
            </select>
            <button className="btn-primary ls-full-button" onClick={directPrint} disabled={busy || selectedTargets.size === 0 || !activeTemplateID || !selectedPrinterID}><Usb size={16} /> Direkt drucken</button>
          </aside>

          <section className="card ls-jobs">
            <div className="ls-section-title"><span>Letzte Druckaufträge</span><button className="ls-icon-button" onClick={() => loadJobs()}><RefreshCw size={15} /></button></div>
            <div className="ls-job-table">
              {jobs.slice(0, 20).map(job => <div key={job.id}><span>#{job.id}</span><span>{targetLabel(job.target_type)} · {job.target_id}</span><span>{job.printer_name || '—'}</span><span>{job.copies}×</span><span className={statusBadge(job.status)}>{job.status}</span></div>)}
              {jobs.length === 0 && <p className="ls-empty-copy">Noch keine Direktdruck-Aufträge.</p>}
            </div>
          </section>
        </div>
      )}

      {tab === 'printers' && (
        <div className="ls-printer-grid">
          <section className="card ls-printer-list">
            <div className="ls-section-title"><span>Netzwerkdrucker</span><span>{printers.length}</span></div>
            {printers.map(printer => <button key={printer.id} className={printerForm.id === printer.id ? 'active' : ''} onClick={() => setPrinterForm(printer)}><Usb size={18} /><span><strong>{printer.name}</strong><small>{printer.address}:{printer.port} · {printer.dpi} DPI</small></span>{printer.is_default && <span className="badge-info">Standard</span>}{!printer.is_active && <span className="badge-neutral">Inaktiv</span>}</button>)}
            {printers.length === 0 && <p className="ls-empty-copy">Noch kein Druckerprofil angelegt.</p>}
            <button className="btn-action ls-full-button" onClick={() => setPrinterForm(EMPTY_PRINTER)}><Plus size={16} /> Neues Profil</button>
          </section>
          <section className="card ls-printer-form">
            <div className="ls-section-title"><span>{printerForm.id ? 'Drucker bearbeiten' : 'Drucker hinzufügen'}</span>{printerForm.id && <button className="ls-icon-button danger" onClick={() => deletePrinter(printerForm)}><Trash2 size={16} /></button>}</div>
            <label>Name<input className="input" value={printerForm.name} onChange={event => setPrinterForm(current => ({ ...current, name: event.target.value }))} placeholder="Zebra Lager" /></label>
            <label>Treiber<select className="input" value={printerForm.driver} onChange={event => setPrinterForm(current => ({ ...current, driver: event.target.value as 'zpl_tcp' }))}><option value="zpl_tcp">Zebra ZPL über TCP</option></select></label>
            <div className="ls-address-row"><label>IP / Hostname<input className="input" value={printerForm.address} onChange={event => setPrinterForm(current => ({ ...current, address: event.target.value }))} placeholder="192.168.1.50" /></label><label>Port<input className="input" type="number" value={printerForm.port} onChange={event => setPrinterForm(current => ({ ...current, port: Number(event.target.value) }))} /></label></div>
            <label>Auflösung<select className="input" value={printerForm.dpi} onChange={event => setPrinterForm(current => ({ ...current, dpi: Number(event.target.value) as 203 | 300 | 600 }))}><option value={203}>203 DPI</option><option value={300}>300 DPI</option><option value={600}>600 DPI</option></select></label>
            <label className="ls-check"><input type="checkbox" checked={printerForm.is_active} onChange={event => setPrinterForm(current => ({ ...current, is_active: event.target.checked }))} /> Aktiv</label>
            <label className="ls-check"><input type="checkbox" checked={printerForm.is_default} onChange={event => setPrinterForm(current => ({ ...current, is_default: event.target.checked }))} /> Als Standarddrucker verwenden</label>
            <button className="btn-primary ls-full-button" onClick={savePrinter} disabled={busy || !printerForm.name.trim() || !printerForm.address.trim()}><Save size={16} /> Druckerprofil speichern</button>
          </section>
        </div>
      )}
    </div>
  );
}
