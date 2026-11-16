// Main client logic for FIPE UI
document.addEventListener('DOMContentLoaded', () => {
  const typeSel = document.getElementById('typeSelect');
  const brandSel = document.getElementById('brandSelect');
  const modelSel = document.getElementById('modelSelect');
  const yearSel = document.getElementById('yearSelect');
  const resultBox = document.getElementById('resultBox');
  const resPrice = document.getElementById('resPrice');
  const resDesc = document.getElementById('resDesc');
  const resRef = document.getElementById('resRef');
  const fuelCode = document.getElementById('fuelCode');
  const codeFipeEl = document.getElementById('codeFipe');

  const setText = (el, txt) => { if (el) el.innerText = txt }

  const historyMonths = document.getElementById('historyMonths');
  const btnLoadHistory = document.getElementById('btnLoadHistory');
  const historyChartCtx = document.getElementById('historyChart').getContext('2d');
  let chart = null;

  // Local cache to avoid repeated selects during session
  const localCache = { brands: {}, models: {}, years: {} };

  // Builds "/api/<endpoint>?a=1&b=2" with every value URL-encoded.
  const apiURL = (endpoint, params) => `/api/${endpoint}?${new URLSearchParams(params)}`;

  async function fetchJSON(url){
    const res = await fetch(url, {cache: 'no-cache'});
    if(!res.ok) throw new Error(`${res.status} ${res.statusText}`);
    return res.json();
  }

  function resetSelects(...sels){
    sels.forEach(s=>{s.innerHTML='<option value="">Select...</option>'; s.disabled=true});
    resultBox.classList.add('d-none');
  }

  async function loadBrands(){
    const type = typeSel.value;
    if(localCache.brands[type]){
      populateSelect(brandSel, localCache.brands[type]);
      return;
    }
    const data = await fetchJSON(apiURL('brands', {type}));
    localCache.brands[type]=data;
    populateSelect(brandSel, data);
  }

  async function loadModels(){
    resetSelects(modelSel, yearSel);
    const type=typeSel.value; const brandId=brandSel.value; if(!brandId) return;
    const key = `${type}:${brandId}`;
    if(localCache.models[key]){ populateSelect(modelSel, localCache.models[key]); return }
    const data = await fetchJSON(apiURL('models', {type, brandId}));
    localCache.models[key]=data;
    populateSelect(modelSel, data);
  }

  async function loadYears(){
    resetSelects(yearSel);
    const type=typeSel.value; const brandId=brandSel.value; const modelId=modelSel.value; if(!modelId) return;
    const key=`${type}:${brandId}:${modelId}`;
    if(localCache.years[key]){ populateSelect(yearSel, localCache.years[key]); return }
    const data = await fetchJSON(apiURL('years', {type, brandId, modelId}));
    localCache.years[key]=data;
    populateSelect(yearSel, data);
  }

  function populateSelect(el, items){
    el.innerHTML='<option value="">Select...</option>';
    items.forEach(it=>{ const o=document.createElement('option'); o.value=it.code||it.value||it.codeFipe||it.id; o.text=it.name||it.label||it.title; el.appendChild(o)});
    el.disabled=false;
  }

  async function loadPrice(){
    const type=typeSel.value, brandId=brandSel.value, modelId=modelSel.value, yearId=yearSel.value; if(!yearId) return;
    const brandName = brandSel.options[brandSel.selectedIndex].text;
    const modelName = modelSel.options[modelSel.selectedIndex].text;
    try{
      const data = await fetchJSON(apiURL('price', {type, brandId, modelId, yearId}));
      setText(resPrice, data.price || 'N/A');
      setText(resDesc, `${data.brand || brandName} - ${data.model || modelName}`);
      setText(resRef, `Ref: ${data.referenceMonth || ''}`);
      setText(fuelCode, data.fuel ? `Fuel: ${data.fuel}${data.fuelAcronym ? ` (${data.fuelAcronym})` : ''}` : '');
      // always update FIPE code from the price response when available
      setText(codeFipeEl, data.codeFipe ? `FIPE code: ${data.codeFipe}` : '');
      // a new vehicle was selected: drop the chart of the previous one
      if(chart){ chart.destroy(); chart = null; }
      resultBox.classList.remove('d-none');
    }catch(err){
      alert('Failed to load price: '+err.message);
    }
  }

  async function loadHistory(){
    const type=typeSel.value, brandId=brandSel.value, modelId=modelSel.value, yearId=yearSel.value; if(!yearId) return;
    const months = historyMonths.value || 12;
    try{
      const data = await fetchJSON(apiURL('priceHistory', {type, brandId, modelId, yearId, months}));
      const history = data.history || data; // accommodate single-point fallback
      // Build entries with parsed date so we can sort chronologically (oldest -> newest)
      const entries = (history || []).map((item, idx) => {
        const it = (typeof item === 'string') ? { price: item } : item || {};
        const priceStr = it.price || it.value || it.priceFormatted || '';
        const refStr = it.referenceMonth || it.month || it.ref || '';
        // parse reference month into Date (try several formats)
        const parseRefToDate = (s) => {
          if(!s) return null;
          const m1 = s.match(/^(\d{1,2})\/(\d{4})$/);
          if(m1) return new Date(parseInt(m1[2]), parseInt(m1[1]) - 1, 1);
          const m2 = s.match(/^(\d{4})-(\d{1,2})$/);
          if(m2) return new Date(parseInt(m2[1]), parseInt(m2[2]) - 1, 1);
          const pt = s.toLowerCase().match(/(janeiro|fevereiro|mar[cç]o|marco|abril|maio|junho|julho|agosto|setembro|outubro|novembro|dezembro)\s+de\s+(\d{4})/i);
          if(pt) {
            const map = { janeiro:1, fevereiro:2, 'março':3, marco:3, abril:4, maio:5, junho:6, julho:7, agosto:8, setembro:9, outubro:10, novembro:11, dezembro:12 };
            const mon = map[pt[1]] || 1;
            return new Date(parseInt(pt[2]), mon-1, 1);
          }
          return null;
        };
        const date = parseRefToDate(refStr) || new Date(Date.now() - (history.length - idx) * 30*24*3600*1000);
        const num = parseFloat((priceStr||'').replace(/[^0-9.,]/g,'').replace(/\./g,'').replace(/,/g,'.')) || 0;
        return { date, label: refStr || `${('0'+(date.getMonth()+1)).slice(-2)}/${date.getFullYear()}`, value: num, rawPrice: priceStr, codeFipe: it.codeFipe || it.code_fipe || '' };
      });

      // sort ascending by date (oldest -> newest)
      entries.sort((a,b) => a.date - b.date);
      const labels = entries.map(e => e.label);
      const values = entries.map(e => e.value);
      // update FIPE code from the most recent history entry if present (overwrite)
      if(entries.length && codeFipeEl){
        const lastCode = entries[entries.length-1].codeFipe || '';
        if(lastCode) setText(codeFipeEl, `FIPE code: ${lastCode}`);
      }
      if(chart) chart.destroy();
      chart = new Chart(historyChartCtx, {type:'line',data:{labels, datasets:[{label:'Price',data:values,backgroundColor:'rgba(37,99,235,0.2)',borderColor:'#2563eb'}]}});
    }catch(err){
      alert('Failed to load history: '+err.message);
    }
  }

  // Events
  // Surface network/API errors instead of leaving unhandled promise rejections.
  const withErrors = (fn, what) => () => fn().catch(err => alert(`Failed to load ${what}: ${err.message}`));
  typeSel.addEventListener('change', ()=>{ resetSelects(brandSel, modelSel, yearSel); withErrors(loadBrands, 'brands')() });
  brandSel.addEventListener('change', withErrors(loadModels, 'models'));
  modelSel.addEventListener('change', withErrors(loadYears, 'years'));
  yearSel.addEventListener('change', loadPrice);
  btnLoadHistory.addEventListener('click', loadHistory);

  // Theme toggle
  const themeToggle = document.getElementById('themeToggle');
  const setTheme = (day)=>{ document.body.classList.toggle('theme-day', !!day); localStorage.setItem('theme-day', day? '1':'0') };
  themeToggle.addEventListener('change', e=> setTheme(e.target.checked));
  setTheme(localStorage.getItem('theme-day') === '1');

  // initial load
  loadBrands().catch(err=>console.error(err));
});
