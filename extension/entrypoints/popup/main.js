function formatDuration(totalSeconds) {
    if (totalSeconds === 0 || isNaN(totalSeconds)) return "0m";
    const days = Math.floor(totalSeconds / (3600 * 24));
    const hours = Math.floor((totalSeconds % (3600 * 24)) / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);

    if (days > 0) return `${days}d ${hours}h`;
    if (hours > 0) return `${hours}h ${minutes}m`;
    return `${minutes}m`;
}

function setOfflineState() {
    const statusInd = document.getElementById("status-indicator");
    statusInd.classList.add("offline");
    
    const statusDot = document.getElementById("status-dot");
    statusDot.classList.remove("pulse");
    
    document.getElementById("status-text").innerText = "Daemon Offline";
    
    document.getElementById("streak-time").innerText = "--";
    document.getElementById("total-events").innerText = "--";
    document.getElementById("event-list").innerHTML = `<li class="empty-state">Unable to connect to daemon</li>`;
}

function setOnlineState() {
    const statusInd = document.getElementById("status-indicator");
    statusInd.classList.remove("offline");
    
    const statusDot = document.getElementById("status-dot");
    if (!statusDot.classList.contains("pulse")) {
        statusDot.classList.add("pulse");
    }
    
    document.getElementById("status-text").innerText = "Daemon Connected";
}

async function fetchStats() {
    try {
        const res = await fetch("http://127.0.0.1:36287/stats");
        if (!res.ok) throw new Error("Daemon offline");
        const data = await res.json();

        setOnlineState();

        // Update Stats
        document.getElementById("streak-time").innerText = formatDuration(data.streak_seconds);
        document.getElementById("total-events").innerText = data.total_events || 0;

        // Update Recent Events
        const eventList = document.getElementById("event-list");
        if (data.recent_events && data.recent_events.length > 0) {
            eventList.innerHTML = "";
            const reversedEvents = [...data.recent_events].reverse();
            reversedEvents.forEach(event => {
                const li = document.createElement("li");
                li.className = "event-item";
                
                const domainSpan = document.createElement("span");
                domainSpan.className = "event-domain";
                domainSpan.innerText = event.domain;

                li.appendChild(domainSpan);
                eventList.appendChild(li);
            });
        } else {
            eventList.innerHTML = `<li class="empty-state">No recent blocks</li>`;
        }

    } catch (err) {
        setOfflineState();
    }
}

// Initial fetch
fetchStats();

// Refresh stats every second to keep the streak timer updated
setInterval(fetchStats, 1000);
