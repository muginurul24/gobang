import { browser } from '$app/environment';

export type ExportColumn<T> = {
  label: string;
  value: (row: T) => unknown;
};

function normalizeValue(value: unknown) {
  if (value === null || value === undefined) {
    return '';
  }

  if (value instanceof Date) {
    return value.toISOString();
  }

  if (typeof value === 'object') {
    return JSON.stringify(value);
  }

  return String(value);
}

function sanitizeFileName(fileName: string) {
  return (
    fileName
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9-_]+/g, '-')
      .replace(/^-+|-+$/g, '') || 'export'
  );
}

function triggerDownload(blob: Blob, fileName: string) {
  if (!browser) {
    return;
  }

  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = fileName;
  anchor.click();
  URL.revokeObjectURL(url);
}

function toSheetRows<T>(columns: ExportColumn<T>[], rows: T[]) {
  return rows.map((row) =>
    Object.fromEntries(
      columns.map((column) => [
        column.label,
        normalizeValue(column.value(row)),
      ]),
    ),
  );
}

export function exportRowsToCSV<T>(
  fileName: string,
  columns: ExportColumn<T>[],
  rows: T[],
) {
  const header = columns.map((column) => column.label);
  const lines = rows.map((row) =>
    columns
      .map((column) => {
        const value = normalizeValue(column.value(row)).replace(/"/g, '""');
        return `"${value}"`;
      })
      .join(','),
  );
  const csv = [header.join(','), ...lines].join('\n');

  triggerDownload(
    new Blob([csv], { type: 'text/csv;charset=utf-8' }),
    `${sanitizeFileName(fileName)}.csv`,
  );
}

export async function exportRowsToXLSX<T>(
  fileName: string,
  sheetName: string,
  columns: ExportColumn<T>[],
  rows: T[],
) {
  const XLSX = await import('xlsx');
  const workbook = XLSX.utils.book_new();
  const worksheet = XLSX.utils.json_to_sheet(toSheetRows(columns, rows));

  XLSX.utils.book_append_sheet(
    workbook,
    worksheet,
    sheetName.slice(0, 31) || 'Sheet1',
  );
  XLSX.writeFile(workbook, `${sanitizeFileName(fileName)}.xlsx`);
}

export async function exportRowsToPDF<T>(
  fileName: string,
  title: string,
  columns: ExportColumn<T>[],
  rows: T[],
) {
  const [{ default: jsPDF }, { default: autoTable }] = await Promise.all([
    import('jspdf'),
    import('jspdf-autotable'),
  ]);
  const document = new jsPDF({
    orientation: columns.length > 5 ? 'landscape' : 'portrait',
    unit: 'pt',
    format: 'a4',
  });

  document.setFontSize(15);
  document.text(title, 40, 42);
  document.setFontSize(9);
  document.text(`Generated ${new Date().toLocaleString('id-ID')}`, 40, 58);

  autoTable(document, {
    startY: 76,
    head: [columns.map((column) => column.label)],
    body: rows.map((row) =>
      columns.map((column) => normalizeValue(column.value(row))),
    ),
    styles: {
      fontSize: 8,
      cellPadding: 6,
      textColor: [23, 20, 15],
    },
    headStyles: {
      fillColor: [16, 34, 26],
      textColor: [248, 251, 248],
    },
    alternateRowStyles: {
      fillColor: [248, 244, 235],
    },
    margin: { left: 32, right: 32 },
  });

  document.save(`${sanitizeFileName(fileName)}.pdf`);
}
