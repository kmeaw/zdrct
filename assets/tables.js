(() => {
  const theadRow = document.getElementById('header-row');
  const tbody = document.getElementById('table-body');
  const addRowBtn = document.getElementById('add-row');
  const addColBtn = document.getElementById('add-col');
  const tableSaveForm = document.getElementById('table-save-form');
  const tableEditor = document.getElementById('table-editor');

  const getHeaderCells = () => Array.from(theadRow.children);

  function createHeaderCell(title = 'Column', isEditable = true) {
    const th = document.createElement('th');
    th.className = 'col-header';
    th.draggable = true;

    const span = document.createElement('span');
    span.className = 'col-title';
    span.textContent = title;
    if (isEditable) span.contentEditable = 'true';
    th.appendChild(span);

    if (getHeaderCells().length >= 2) {
      const btn = document.createElement('button');
      btn.className = 'remove-col-btn';
      btn.title = 'Remove column';
      btn.innerText = '🗑️';
      th.appendChild(btn);
    }

    return th;
  }

  function addRow() {
    const headerCells = getHeaderCells();
    const tr = document.createElement('tr');
    tr.draggable = true;

    headerCells.forEach((th, i) => {
      const td = document.createElement('td');

      if (th.classList.contains('handle-header')) {
        td.className = 'handle';
        td.textContent = '☰';
      } else {
        td.contentEditable = 'true';

        if (i === 1) {
          td.textContent = (tbody.rows.length + 1);
        } else {
          td.textContent = '';
        }
      }
      tr.appendChild(td);
    });

    const removeCell = document.createElement('td');
    removeCell.className = 'remove-row';
    const rb = document.createElement('button');
    rb.className = 'remove-row-btn';
    rb.title = 'Remove row';
    rb.innerText = '🗑️';
    removeCell.appendChild(rb);
    tr.appendChild(removeCell);

    tbody.appendChild(tr);
    return tr;
  }

  (function ensureRemoveButtons() {
    Array.from(tbody.rows).forEach(row => {
      const last = row.cells[row.cells.length - 1];
      if (!last || !last.classList.contains('remove-row')) {
        const removeCell = document.createElement('td');
        removeCell.className = 'remove-row';
        const rb = document.createElement('button');
        rb.className = 'remove-row-btn';
        rb.title = 'Remove row';
        rb.innerText = '🗑️';
        removeCell.appendChild(rb);
        row.appendChild(removeCell);
      }
      if (!row.cells[0] || !row.cells[0].classList.contains('handle')) {
        const handleCell = document.createElement('td');
        handleCell.className = 'handle';
        handleCell.textContent = '☰';
        row.insertBefore(handleCell, row.cells[0] || null);
      }
      Array.from(row.cells).forEach(cell => {
        if (cell.classList.contains('handle') || cell.classList.contains('remove-row')) return;
        cell.contentEditable = 'true';
      });
    });
  })();

  theadRow.addEventListener('click', (e) => {
    const btn = e.target.closest('.remove-col-btn');
    if (!btn) return;
    const th = btn.closest('th');
    if (!th) return;
    const headers = getHeaderCells();
    const idx = headers.indexOf(th);
    if (idx <= 2) return;
    th.remove();
    Array.from(tbody.rows).forEach(row => {
      if (row.cells[idx]) row.deleteCell(idx);
    });
  });

  addColBtn.addEventListener('click', () => {
    const th = createHeaderCell('Column_' + (theadRow.cells.length - 1), true);
    theadRow.appendChild(th);
    Array.from(tbody.rows).forEach(row => {
      const td = document.createElement('td');
      td.contentEditable = 'true';
      row.insertBefore(td, row.cells[row.cells.length - 1]);
    });
  });

  addRowBtn.addEventListener('click', () => {
    const newRow = addRow();
    const editable = Array.from(newRow.querySelectorAll('td[contenteditable="true"]'))[newRow.cells.length > 1 ? 1 : 0];
    if (editable) focusAndSelect(editable, true);
  });

  tbody.addEventListener('click', (e) => {
    const btn = e.target.closest('.remove-row-btn');
    if (!btn) return;
    const tr = btn.closest('tr');
    if (!tr) return;
    tr.remove();
  });

  let draggedRow = null;
  let rowDragAllowed = false;

  tbody.addEventListener('mousedown', (e) => {
    if (e.target.closest('.handle')) rowDragAllowed = true;
  });
  document.addEventListener('mouseup', () => rowDragAllowed = false);

  tbody.addEventListener('dragstart', (e) => {
    if (!rowDragAllowed) { e.preventDefault(); return; }
    draggedRow = e.target.closest('tr');
    e.dataTransfer.effectAllowed = 'move';
    // Firefox requires data to be set on dragstart
    e.dataTransfer.setData('text/plain', '');
    draggedRow.classList.add('dragging');
  });

  tbody.addEventListener('dragover', (e) => {
    e.preventDefault();
    const tr = e.target.closest('tr');
    if (!tr || tr === draggedRow) return;
    tr.classList.add('tr-drag-over');
  });

  tbody.addEventListener('dragleave', (e) => {
    const tr = e.target.closest('tr');
    if (tr) tr.classList.remove('tr-drag-over');
  });

  tbody.addEventListener('drop', (e) => {
    e.preventDefault();
    const tr = e.target.closest('tr');
    if (!tr || !draggedRow || tr === draggedRow) return;
    tr.classList.remove('tr-drag-over');

    const rect = tr.getBoundingClientRect();
    const insertAfter = (e.clientY - rect.top) > rect.height / 2;

    if (insertAfter) tr.after(draggedRow);
    else tr.before(draggedRow);

    draggedRow.classList.remove('dragging');
    draggedRow = null;
    rowDragAllowed = false;
  });

  tbody.addEventListener('dragend', () => {
    if (draggedRow) draggedRow.classList.remove('dragging');
    draggedRow = null;
    rowDragAllowed = false;
    Array.from(tbody.rows).forEach(r => r.classList.remove('tr-drag-over'));
  });

  let draggedTh = null;
  theadRow.addEventListener('dragstart', (e) => {
    const th = e.target.closest('th');
    if (!th || th.classList.contains('handle-header') || th.classList.contains('col-id')) { e.preventDefault(); return; }
    draggedTh = th;
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', '');
  });

  theadRow.addEventListener('dragover', (e) => {
    e.preventDefault();
    const th = e.target.closest('th');
    if (!th || th === draggedTh || th.classList.contains('handle-header') || th.classList.contains('col-id')) return;
    th.classList.add('th-drag-over');
  });

  theadRow.addEventListener('dragleave', (e) => {
    const th = e.target.closest('th');
    if (th) th.classList.remove('th-drag-over');
  });

  theadRow.addEventListener('drop', (e) => {
    e.preventDefault();
    const targetTh = e.target.closest('th');
    if (!targetTh || !draggedTh || targetTh === draggedTh || targetTh.classList.contains('handle-header') || targetTh.classList.contains('col-id')) return;

    const headers = getHeaderCells();
    const draggedIndex = headers.indexOf(draggedTh);
    const targetIndex = headers.indexOf(targetTh);

    const rect = targetTh.getBoundingClientRect();
    const insertBefore = (e.clientX - rect.left) < rect.width / 2;

    if (insertBefore) theadRow.insertBefore(draggedTh, targetTh);
    else theadRow.insertBefore(draggedTh, targetTh.nextSibling);

    Array.from(tbody.rows).forEach(row => {
      const cells = Array.from(row.cells);
      const draggedCell = cells[draggedIndex];
      const targetCell = cells[targetIndex];
      if (!draggedCell || !targetCell) return;
      if (insertBefore) row.insertBefore(draggedCell, targetCell);
      else row.insertBefore(draggedCell, targetCell.nextSibling);
    });

    Array.from(theadRow.children).forEach(ch => ch.classList.remove('th-drag-over'));
    draggedTh = null;
  });

  theadRow.addEventListener('dragend', () => {
    draggedTh = null;
    Array.from(theadRow.children).forEach(ch => ch.classList.remove('th-drag-over'));
  });

  function focusAndSelect(cell, selectAll = false) {
    cell.focus();
    const sel = window.getSelection();
    sel.removeAllRanges();
    const range = document.createRange();
    if (selectAll) {
      range.selectNodeContents(cell);
      if (!cell.textContent) {
        range.setStart(cell, 0);
        range.collapse(true);
      }
    } else {
      const len = cell.textContent.length;
      if (cell.firstChild) range.setStart(cell.firstChild, Math.min(len, (cell.firstChild.length || len)));
      else range.setStart(cell, 0);
      range.collapse(true);
    }
    sel.addRange(range);
  }

  function editableCellsOfRow(row) {
    return Array.from(row.querySelectorAll('td[contenteditable="true"]'));
  }

  function getCaretOffsetWithinCell() {
    const sel = window.getSelection();
    if (!sel || !sel.rangeCount) return { node: null, offset: 0 };
    const range = sel.getRangeAt(0);
    let node = range.startContainer;
    let offset = range.startOffset;

    return { node, offset };
  }

  tbody.addEventListener('keydown', (e) => {
    const cell = e.target.closest('td');
    if (!cell || !cell.isContentEditable) return;

    if (e.key === 'Enter') e.preventDefault();

    const tr = cell.parentElement;
    const rows = Array.from(tbody.rows);
    const rowIndex = rows.indexOf(tr);
    const editableCells = editableCellsOfRow(tr);
    const cellIndex = editableCells.indexOf(cell);

    // caret info
    const sel = window.getSelection();
    const caret = getCaretOffsetWithinCell();
    const textLen = cell.textContent.length;
    const caretOffset = caret.offset ?? 0;

    const moveToCell = (targetRowIndex, targetCellIndex, selectAll = false) => {
      const rs = Array.from(tbody.rows);
      const safeRowIndex = Math.max(0, Math.min(targetRowIndex, rs.length - 1));
      const targetRow = rs[safeRowIndex];
      if (!targetRow) return;
      const targetEditable = editableCellsOfRow(targetRow)[targetCellIndex];
      if (!targetEditable) return;
      focusAndSelect(targetEditable, selectAll);
    };

    switch (e.key) {
      case 'ArrowLeft':
        if (sel.isCollapsed && caretOffset === 0) {
          e.preventDefault();
          if (cellIndex > 0) {
            moveToCell(rowIndex, cellIndex - 1, false);
          } else if (rowIndex > 0) {
            const prevRow = rows[rowIndex - 1];
            const prevEditable = editableCellsOfRow(prevRow);
            moveToCell(rowIndex - 1, prevEditable.length - 1, false);
          }
        }
        break;

      case 'ArrowRight':
        if (sel.isCollapsed && caretOffset >= textLen) {
          e.preventDefault();
          const thisRowEditable = editableCellsOfRow(tr);
          if (cellIndex < thisRowEditable.length - 1) {
            moveToCell(rowIndex, cellIndex + 1, false);
          } else if (rowIndex < rows.length - 1) {
            moveToCell(rowIndex + 1, 0, false);
          }
        }
        break;

      case 'ArrowUp':
        e.preventDefault();
        if (rowIndex > 0) moveToCell(rowIndex - 1, cellIndex, false);
        break;

      case 'ArrowDown':
        e.preventDefault();
        if (rowIndex < rows.length - 1) moveToCell(rowIndex + 1, cellIndex, false);
        break;

      case 'Enter':
        e.preventDefault();

        let rowsNow = Array.from(tbody.rows);
        let nextRowIndex = rowIndex;
        let nextCellIndex = cellIndex + 1;
        const currentRowEditable = editableCellsOfRow(tr);

        if (nextCellIndex >= currentRowEditable.length) {
          nextCellIndex = currentRowEditable.length > 1 ? 1 : 0;
          nextRowIndex = rowIndex + 1;
          if (nextRowIndex >= rowsNow.length) {
            const newRow = addRow();
            rowsNow = Array.from(tbody.rows);
            nextRowIndex = rowsNow.length - 1;
            const targetEditable = editableCellsOfRow(rowsNow[nextRowIndex])[nextCellIndex];
            if (targetEditable) {
              focusAndSelect(targetEditable, true);
              return;
            }
          }
        }

        rowsNow = Array.from(tbody.rows);
        const targetRow = rowsNow[nextRowIndex] || rowsNow[rowsNow.length - 1];
        const targetEditable = targetRow ? editableCellsOfRow(targetRow)[nextCellIndex] : null;
        if (targetEditable) {
          focusAndSelect(targetEditable, true);
        }
        break;
    }
  });

  tbody.addEventListener('keypress', (e) => {
    if (e.key === 'Enter' && e.target.closest('td') && e.target.closest('td').isContentEditable) {
      e.preventDefault();
    }
  });

  tbody.addEventListener('focusin', (e) => {
    const td = e.target.closest('td');
    if (!td || !td.isContentEditable) return;
  });

  tableSaveForm.addEventListener('submit', async (e) => {
    e.preventDefault();

    const formData = new FormData(tableSaveForm);
    const data = {
      table: {
        Columns: [],
        Data: []
      }
    };

    formData.forEach((value, key) => {
      data[key] = value;
    });

    theadRow.querySelectorAll('span.col-title').forEach((s, i) => {
      if (i > 0) {
        data.table.Columns.push({'Name': s.textContent});
      }
    });

    data.table.Data = Array.from(tbody.rows)
      .sort((a,b) => parseInt(a.cells[1]) - parseInt(b.cells[1]))
      .map((r) => Array.from(r.cells).slice(2, -1).map(c => c.textContent));

    const resp = await fetch(tableSaveForm.action, {
      method: 'POST',
      body: JSON.stringify(data),
      headers: {
        'Content-Type': 'application/json'
      }
    });
    if (!resp.ok) {
      tableSaveForm.querySelector('input[type=submit]').value = await resp.text();
      return false;
    }

    tableEditor.innerHTML = await resp.text();
    tableEditor.querySelectorAll('script').forEach($s0 => {
      const $s1 = document.createElement('script');
      Array.from($s0.attributes).forEach(a => {
        $s1.setAttribute(a.name, a.value);
      });
      $s1.textContent = $s0.textContent;
      $s0.parentNode.replaceChild($s1, $s0);
    });
  });
})();
