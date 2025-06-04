const { useState, useEffect, useMemo } = React;

function App() {
  const [darkMode, setDarkMode] = useState(() => {
    return localStorage.getItem('darkMode') === 'true';
  });
  const [albums, setAlbums] = useState([]);
  const [filter, setFilter] = useState('');
  const [modalAlbum, setModalAlbum] = useState(null);
  const [actionMode, setActionMode] = useState(null);
  const [toasts, setToasts] = useState([]);
  const [currentPage, setCurrentPage] = useState(1);
  const pageSize = 50;

  useEffect(() => {
    document.body.style.backgroundColor = darkMode ? '#2d3748' : '#edf2f7';
  }, [darkMode]);

  useEffect(() => {
    localStorage.setItem('darkMode', darkMode);
  }, [darkMode]);

  const fetchAlbums = async () => {
    try {
      const res = await fetch('/api/v1/albums');
      const data = await res.json();
      setAlbums(data);
      setCurrentPage(1);
    } catch (e) {
      addToast('Error fetching albums');
    }
  };
  useEffect(fetchAlbums, []);

  useEffect(() => {
    setCurrentPage(1);
  }, [filter]);

  const filtered = useMemo(() => {
    const toks = filter.toLowerCase().trim().split(/\s+/).filter(Boolean);
    return albums.filter(a => toks.every(tok => tok.startsWith('r=')
      ? a.rating.toString().includes(tok.slice(2))
      : a.albumartist.toLowerCase().includes(tok) || a.album.toLowerCase().includes(tok)
    ));
  }, [albums, filter]);

  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const paged = filtered.slice((currentPage - 1) * pageSize, currentPage * pageSize);

  const addToast = msg => {
    const id = Date.now();
    setToasts(t => [...t, { id, msg }]);
    setTimeout(() => setToasts(t => t.filter(x => x.id !== id)), 3000);
  };

  // Updated callApi with optional pre-toast message
  const callApi = async (url, method, body, preToastMsg) => {
    if (preToastMsg) addToast(preToastMsg);
    try {
      const res = await fetch(url, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
      const json = await res.json();
      addToast(json.message || json.error || 'Done');
      fetchAlbums();
    } catch (e) {
      addToast('Error');
    }
  };

  const commitAction = (albumId, mode, ratingValue) => {
    if (mode === 'rate') {
      const payload = ratingValue === 'delete' ? { rating: 'Delete' } : { rating: ratingValue.toString() };
      callApi(`/api/v1/albums/${albumId}/rating`, 'POST', payload);
    } else {
      callApi(`/api/v1/playlist/add/album/${albumId}`, 'POST', { mode });
    }
    setModalAlbum(null);
    setActionMode(null);
  };

  const bgPage = darkMode ? 'bg-gray-800 text-gray-100' : 'bg-gray-100 text-gray-900';
  const bgCard = darkMode ? 'bg-gray-700' : 'bg-white';
  const borderColor = darkMode ? 'border-gray-600' : 'border-gray-300';
  const hoverRow = darkMode ? 'hover:bg-gray-600' : 'hover:bg-gray-200';

  return (
    <div className={`${bgPage} h-full flex flex-col px-2 sm:px-4 py-4 font-sans`}>
      <div className="mb-4">
        <div className="flex justify-between items-center mb-2 sm:hidden">
          <button
            onClick={() => callApi('/api/v1/playback/random/album', 'POST', null, 'Playing random album...')}
            className="px-3 py-1 bg-blue-600 text-white rounded hover:bg-blue-700"
          >
            Random Album
          </button>
          <button
            onClick={() => callApi('/api/v1/playback/random/tracks', 'POST', null, 'Playing random tracks...')}
            className="px-3 py-1 bg-green-600 text-white rounded hover:bg-green-700"
          >
            Random Tracks
          </button>
          <button onClick={() => setDarkMode(d => !d)} className="p-2 rounded">{darkMode ? '☀️' : '🌙'}</button>
        </div>
        <div className="hidden sm:flex flex-row items-center gap-2 mb-2">
          <button
            onClick={() => callApi('/api/v1/playback/random/album', 'POST', null, 'Playing random album...')}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700"
          >
            Random Album
          </button>
          <button
            onClick={() => callApi('/api/v1/playback/random/tracks', 'POST', null, 'Playing random tracks...')}
            className="px-4 py-2 bg-green-600 text-white rounded hover:bg-green-700"
          >
            Random Tracks
          </button>
          <button onClick={() => setDarkMode(d => !d)} className="ml-auto p-2 rounded">{darkMode ? '☀️' : '🌙'}</button>
        </div>
        <div>
          <input
            value={filter}
            onChange={e => setFilter(e.target.value)}
            placeholder="Search artist, album or r=rating"
            className={`w-full p-2 border rounded focus:outline-none focus:ring-2 focus:ring-blue-500 ${bgCard} border ${borderColor}`}
          />
        </div>
      </div>

      {/* Desktop Table */}
      <div className="hidden sm:flex flex-col flex-1 min-h-0">
        <div className={`flex-1 overflow-y-auto overflow-x-auto rounded-lg shadow ${bgCard} border ${borderColor}`}>
          <table className="min-w-full border-collapse">
            <thead>
              <tr className={`${darkMode ? 'bg-gray-700' : 'bg-gray-200'}`}>
                <th className={`px-4 py-2 text-left border-b ${borderColor}`}>Artist</th>
                <th className={`px-4 py-2 text-left border-b ${borderColor}`}>Album</th>
                <th className={`px-4 py-2 text-left w-40 border-b ${borderColor}`}>Rating</th>
              </tr>
            </thead>
            <tbody>
              {paged.map(a => {
                const r = parseInt(a.rating, 10);
                const filled = !isNaN(r);
                return (
                  <tr key={a.id} onClick={() => { setModalAlbum(a); setActionMode(null); }} className={`cursor-pointer ${hoverRow}`}>
                    <td className={`px-4 py-2 border-b ${borderColor}`}>{a.albumartist}</td>
                    <td className={`px-4 py-2 border-b ${borderColor}`}>{a.album}</td>
                    <td className={`px-4 py-2 border-b ${borderColor}`}>
                      <div className="relative inline-block align-middle" style={{ width: '100px', height: '8px' }}>
                        <div className="absolute inset-0 rounded-full" style={{ backgroundColor: darkMode ? '#4a5568' : '#e2e8f0' }} />
                        {filled && <div className="absolute inset-y-0 left-0 rounded-full" style={{ width: `${r * 10}%`, backgroundColor: darkMode ? '#63b3ed' : '#3182ce' }} />}
                      </div>
                      <span className="ml-2 align-middle text-sm">{filled ? r : '–'}</span>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
        <div className="flex justify-center items-center gap-2 p-2 border-t">
          <button onClick={() => setCurrentPage(p => Math.max(p - 1, 1))} disabled={currentPage === 1} className={`px-3 py-1 border rounded ${borderColor}`}>Prev</button>
          <span>Page {currentPage} of {totalPages}</span>
          <button onClick={() => setCurrentPage(p => Math.min(p + 1, totalPages))} disabled={currentPage === totalPages} className={`px-3 py-1 border rounded ${borderColor}`}>Next</button>
        </div>
      </div>

      {/* Mobile List */}
      <div className="sm:hidden space-y-3">
        {paged.map(a => {
          const r = parseInt(a.rating, 10);
          const filled = !isNaN(r);
          return (
            <div key={a.id} onClick={() => { setModalAlbum(a); setActionMode(null); }} className={`p-4 rounded-lg shadow ${bgCard} border ${borderColor} ${hoverRow}`}>
              <div className="font-semibold">{a.albumartist}</div>
              <div className="text-sm mb-2">{a.album}</div>
              <div className="flex items-center">
                <div className="relative inline-block" style={{ width: '80px', height: '6px' }}>
                  <div className="absolute inset-0 rounded-full" style={{ backgroundColor: darkMode ? '#4a5568' : '#e2e8f0' }} />
                  {filled && <div className="absolute inset-y-0 left-0 rounded-full" style={{ width: `${r * 10}%`, backgroundColor: darkMode ? '#63b3ed' : '#3182ce' }} />}
                </div>
                <span className="ml-2 text-xs">{filled ? r : '–'}</span>
              </div>
            </div>
          );
        })}
      </div>

      {/* Mobile Pagination */}
      <div className="sm:hidden flex justify-center items-center gap-2 p-2">
        <button onClick={() => setCurrentPage(p => Math.max(p - 1, 1))} disabled={currentPage === 1} className={`px-3 py-1 border rounded ${borderColor}`}>Prev</button>
        <span>Page {currentPage} of {totalPages}</span>
        <button onClick={() => setCurrentPage(p => Math.min(p + 1, totalPages))} disabled={currentPage === totalPages} className={`px-3 py-1 border rounded ${borderColor}`}>Next</button>
      </div>

      {/* Toasts */}
      <div className="fixed bottom-4 right-4 space-y-2">
        {toasts.map(t => (<div key={t.id} className="px-4 py-2 bg-black bg-opacity-80 text-white rounded">{t.msg}</div>))}
      </div>

      {/* Modal */}
      {modalAlbum && (
        <div className="fixed inset-0 flex items-center justify-center" style={{ backgroundColor: darkMode ? 'rgba(45,55,72,0.8)' : 'rgba(237,242,247,0.8)' }}>
          <div className={`p-6 rounded-lg shadow ${bgCard} border ${borderColor} max-w-sm w-full`}>
            <h2 className="text-xl font-semibold mb-4">{modalAlbum.albumartist} – {modalAlbum.album}</h2>
            {actionMode === 'rate' ? (
              <div className="grid grid-cols-2 gap-2 mb-3">
                {Array.from({ length: 10 }, (_, i) => i + 1).map(num => (
                  <button key={num} onClick={() => commitAction(modalAlbum.id, 'rate', num)} className={`px-3 py-2 rounded ${darkMode ? 'bg-blue-600 text-white hover:bg-blue-500' : 'bg-blue-100 text-gray-900 hover:bg-blue-200'}`}>{num}</button>
                ))}
                <button onClick={() => commitAction(modalAlbum.id, 'rate', 'delete')} className={`px-3 py-2 rounded ${darkMode ? 'bg-red-600 text-white hover:bg-red-500' : 'bg-red-100 text-gray-900 hover:bg-red-200'}`}>Delete</button>
              </div>
            ) : (
              <div className="space-y-3 mb-3">
                {['add', 'insert', 'replace', 'rate'].map(act => (
                  <button key={act} onClick={() => { if (act === 'rate') setActionMode(act); else commitAction(modalAlbum.id, act); }} className={`block w-full text-left px-3 py-2 rounded ${darkMode ? 'bg-blue-600 text-white hover:bg-blue-500' : 'bg-blue-100 text-gray-900 hover:bg-blue-200'}`}>{act.charAt(0).toUpperCase() + act.slice(1)}</button>
                ))}
              </div>
            )}
            <button onClick={() => { setModalAlbum(null); setActionMode(null); }} className={`mt-2 text-sm rounded px-2 py-1 ${darkMode ? 'text-gray-200 hover:bg-gray-600' : 'text-gray-700 hover:bg-gray-200'}`}>Close</button>
          </div>
        </div>
      )}
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<App />);

