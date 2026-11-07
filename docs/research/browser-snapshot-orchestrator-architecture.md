# Especificación de Arquitectura de Software y Documento de Requisitos de Producto: Orquestador Universal de Captura y Verificación Visual Multiplataforma

> Convertido a Markdown el 2026-11-06 desde `Browser Snapshot Orchestrator Architecture.docx` (investigación de Gemini; el original sigue en el vault de Obsidian). Las citas son notas al pie; las 26 ecuaciones, que en el original eran imágenes, se transcribieron a LaTeX. Investigación generada por IA sin verificar; ver [Idea Refinement](../idea-refinement.md) para errores conocidos.

## Fundamentos del Sistema y Paradigma Arquitectónico

Las suites consolidadas de automatización de pruebas como Playwright, Puppeteer y Selenium introducen una sobrecarga de infraestructura considerable cuando el objetivo técnico se circunscribe a la verificación de regresión visual y compatibilidad de maquetación entre navegadores[^1]. Estas herramientas distribuyen capas complejas de abstracción para la emulación de interacciones de usuario, despachadores de eventos sintéticos, proxies de red intermedios y paquetes binarios de navegadores completos que consumen varios gigabytes de almacenamiento secundario[^1]. En entornos de integración continua y estaciones de desarrollo locales, esta carga impone tiempos de descarga elevados, problemas de compilación cruzada y un consumo desproporcionado de memoria[^1].

Un orquestador de capturas visuales autónomo y sin configuración previa (zero-config) resuelve esta ineficiencia desacoplando la capa de automatización de las distribuciones propietarias de navegadores[^2]. En lugar de suministrar compilaciones binarias empaquetadas, el sistema inspecciona el sistema operativo anfitrión (Windows, macOS o Linux), localiza todos los navegadores instalados de manera dinámica y se conecta de forma directa a sus puertos nativos de depuración y control remoto[^2].

La arquitectura prescinde de controladores intermedios pesados como chromedriver o geckodriver, interactuando directamente a través de conexiones de red nativas: Chrome DevTools Protocol (CDP) mediante WebSockets para motores Blink, el estándar W3C WebDriver BiDi sobre WebSockets para motores Gecko, y la interfaz REST de W3C WebDriver Classic para WebKit en macOS y Linux[^5]. Esta reducción técnica minimiza el paquete distribuido a un cliente de red y un motor analítico de imágenes, manteniendo la fidelidad visual absoluta que solo los binarios del sistema operativo pueden reproducir[^2].

| **Familia de Motor** | **Navegadores Comerciales Representativos** | **Protocolo de Control** | **Capa de Transporte** | **Proceso Controlador Requerido** | **Modo de Rasterizado de Documento** |
|---|---|---|---|---|---|
| **Blink / V8** | Google Chrome, Chromium, Microsoft Edge, Brave, Opera, Opera GX, Vivaldi, Arc | Chrome DevTools Protocol (CDP)[^8] | JSON-RPC 2.0 sobre WebSocket[^5] | Ninguno (comunicación directa con el binario)[^8] | Pasada única nativa vía Page.captureScreenshot con captureBeyondViewport[^8] |
| **Gecko / SpiderMonkey** | Mozilla Firefox, Firefox Developer Edition, Nightly, Floorp, Waterfox, LibreWolf | W3C WebDriver BiDi[^6] | JSON-RPC 2.0 sobre WebSocket (/session)[^7] | Ninguno (Remote Agent interno en --remote-debugging-port)[^3] | Pasada única nativa vía browsingContext.captureScreenshot con origin: "document"[^5] |
| **WebKit / JavaScriptCore** | Apple Safari, Safari Technology Preview | W3C WebDriver Classic[^17] | REST API sobre HTTP/1.1 (/session/{id}/...)[^18] | Binario del sistema /usr/bin/safaridriver[^17] | Algoritmo de desplazamiento y cosido (scroll-and-stitch) con supresión de capas fijas[^2] |
| **WebKitGTK** | GNOME Web (Epiphany) | W3C WebDriver Classic[^9] | REST API sobre HTTP/1.1[^9] | Binario del sistema WebKitWebDriver[^9] | Segmentación de viewport y composición matricial en búfer de imagen[^9] |

## Descubrimiento Determinista de Binarios en Windows, macOS y Linux

La ejecución con cero configuración exige que el orquestador identifique y catalogue exhaustivamente los navegadores presentes en el entorno anfitrión en menos de trescientos milisegundos sin requerir parámetros de ruta explícitos por parte del desarrollador.

En los sistemas operativos Windows, el registro centraliza la inscripción de clientes de Internet. La rutina de escaneo del orquestador realiza lecturas asíncronas no bloqueantes sobre la colmena HKEY_LOCAL_MACHINE\SOFTWARE\Clients\StartMenuInternet y su contraparte de ámbito de usuario HKEY_CURRENT_USER\SOFTWARE\Clients\StartMenuInternet[^24]. En arquitecturas de 64 bits que albergan ejecutables de 32 bits, se analiza de forma análoga la subclave redirigida HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432Node\Clients\StartMenuInternet[^4]. La clave StartMenuInternet constituye el estándar formal en Windows donde los instaladores de navegadores como Chrome, Firefox, Opera, Edge, Brave y Vivaldi registran su identidad operativa[^4]. Dentro de cada subclave descubierta, el sistema navega hacia shell\open\command y extrae el valor predeterminado, el cual contiene la ruta física del archivo ejecutable[^28]. Para navegadores portables o instalados en contextos de usuario restringidos que omiten el registro en StartMenuInternet, el sistema aplica una heurística secundaria que consulta HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\\<browser>.exe y rastrea sistemáticamente los árboles de directorios en %ProgramFiles%, %ProgramFiles(x86)% y %LocalAppData%[^29].

En el entorno macOS, el subsistema de descubrimiento interactúa con la base de datos de metadatos de LaunchServices a través de la herramienta de línea de comandos Spotlight /usr/bin/mdfind. Al invocar consultas parametrizadas basadas en el identificador de paquete (kMDItemCFBundleIdentifier), el orquestador localiza instantáneamente el paquete de la aplicación sin importar si el usuario la instaló en la carpeta estándar del sistema /Applications, en el directorio personal ~/Applications o en particiones secundarias. Para la integración con WebKit en macOS, el ejecutable de automatización /usr/bin/safaridriver se encuentra presente de manera predeterminada en el sistema base, mientras que las versiones preliminares de Safari Technology Preview se resuelven mediante el argumento --tp administrado por el mismo controlador[^17].

En distribuciones Linux, la dispersión de empaquetado demanda una estrategia de detección estratificada. El orquestador escanea en primer término los archivos de especificación de escritorio XDG ubicados en /usr/share/applications/\*.desktop y ~/.local/share/applications/\*.desktop, filtrando aquellas entradas que declaran la categoría Network;WebBrowser; y obteniendo el comando de invocación mediante la clave Exec. Para contemplar el software distribuido bajo formatos de contenedores aislados, el sistema ejecuta consultas rápidas contra el gestor Flatpak mediante flatpak list --app --columns=application contra un catálogo estandarizado de identificadores de aplicación, y contra el daemon de Snap ejecutando snap list para identificar instancias instaladas de Firefox, Chromium o derivados[^33]. Si los métodos anteriores no completan el catálogo, la última capa de resolución itera sobre las rutas expuestas en la variable de entorno \$PATH.

| **Navegador Objetivo** | **Familia Técnica** | **macOS (Identificador / Ruta)** | **Windows (Registro / Ruta de Disco)** | **Linux (Binario / Identificador de Paquete)** |
|---|---|---|---|---|
| **Google Chrome** | Blink / CDP[^8] | com.google.Chrome<br>/Applications/Google Chrome.app | Clients\StartMenuInternet\Google Chrome<br>%ProgramFiles%\Google\Chrome\Application\chrome.exe[^30] | google-chrome-stable, google-chrome<br>com.google.Chrome (Flatpak) |
| **Chromium** | Blink / CDP[^1] | org.chromium.Chromium<br>/Applications/Chromium.app | Clients\StartMenuInternet\Chromium<br>%LocalAppData%\Chromium\Application\chrome.exe | chromium, chromium-browser<br>org.chromium.Chromium (Flatpak) |
| **Microsoft Edge** | Blink / CDP[^1] | com.microsoft.edgemac<br>/Applications/Microsoft Edge.app | Clients\StartMenuInternet\Microsoft Edge<br>%ProgramFiles(x86)%\Microsoft\Edge\Application\msedge.exe[^4] | microsoft-edge-stable<br>com.microsoft.Edge (Flatpak) |
| **Brave Browser** | Blink / CDP | com.brave.Browser<br>/Applications/Brave Browser.app | Clients\StartMenuInternet\Brave<br>%LocalAppData%\BraveSoftware\Brave-Browser\Application\brave.exe | brave-browser, brave<br>com.brave.Browser (Flatpak) |
| **Opera** | Blink / CDP[^1] | com.operasoftware.Opera<br>/Applications/Opera.app | Clients\StartMenuInternet\OperaStable<br>%LocalAppData%\Programs\Opera\launcher.exe | opera<br>com.opera.Opera (Flatpak / Snap)[^33] |
| **Opera GX** | Blink / CDP | com.operasoftware.OperaGX<br>/Applications/Opera GX.app | Clients\StartMenuInternet\Opera GXStable<br>%LocalAppData%\Programs\Opera GX\launcher.exe[^38] | No disponible en la plataforma |
| **Vivaldi** | Blink / CDP | com.vivaldi.Vivaldi<br>/Applications/Vivaldi.app | Clients\StartMenuInternet\Vivaldi<br>%LocalAppData%\Vivaldi\Application\vivaldi.exe[^4] | vivaldi, vivaldi-stable<br>com.vivaldi.Vivaldi (Flatpak) |
| **Arc** | Blink / CDP | company.thebrowser.Browser<br>/Applications/Arc.app | Clients\StartMenuInternet\Arc<br>%LocalAppData%\Arc\Arc.exe | No disponible en la plataforma |
| **Mozilla Firefox** | Gecko / BiDi[^6] | org.mozilla.firefox<br>/Applications/Firefox.app[^7] | Clients\StartMenuInternet\FIREFOX.EXE<br>%ProgramFiles%\Mozilla Firefox\firefox.exe[^39] | firefox<br>org.mozilla.firefox (Flatpak / Snap)[^33] |
| **Firefox Developer / Nightly** | Gecko / BiDi[^14] | org.mozilla.firefoxdeveloperedition<br>/Applications/Firefox Nightly.app | Clients\StartMenuInternet\Firefox Developer Edition<br>%ProgramFiles%\Firefox Nightly\firefox.exe | firefox-trunk, firefox-developer-edition |
| **Floorp** | Gecko / BiDi | jp.ablaze.floorp<br>/Applications/Floorp.app | Clients\StartMenuInternet\Floorp<br>%ProgramFiles%\Floorp\floorp.exe | floorp<br>one.ablaze.floorp (Flatpak) |
| **Waterfox** | Gecko / BiDi | net.waterfox.waterfox<br>/Applications/Waterfox.app | Clients\StartMenuInternet\Waterfox<br>%ProgramFiles%\Waterfox\waterfox.exe | waterfox<br>net.waterfox.waterfox (Flatpak) |
| **LibreWolf** | Gecko / BiDi | io.gitlab.librewolf-community<br>/Applications/LibreWolf.app | Clients\StartMenuInternet\LibreWolf<br>%ProgramFiles%\LibreWolf\librewolf.exe | librewolf<br>io.gitlab.librewolf-community (Flatpak) |
| **Apple Safari** | WebKit / W3C[^17] | Controlador del sistema:<br>/usr/bin/safaridriver[^17] | No disponible en la plataforma[^17] | No disponible en la plataforma[^17] |
| **Safari Technology Preview** | WebKit / W3C | Controlador del sistema con bandera:<br>/usr/bin/safaridriver --tp | No disponible en la plataforma | No disponible en la plataforma |
| **WebKitGTK (Epiphany)** | WebKit / W3C[^9] | No disponible en la plataforma | No disponible en la plataforma | Binario del sistema:<br>WebKitWebDriver, epiphany[^9] |

El lanzamiento desatendido de navegadores comerciales dirigidos al consumidor final introduce comportamientos divergentes que deben ser neutralizados en tiempo de ejecución.

En el caso de Brave, su motor interno de bloqueo de elementos publicitarios (Brave Shields) bloquea por defecto fuentes alojadas en redes de distribución de contenido externas y scripts analíticos de terceros, lo que genera desajustes en la maquetación respecto a Google Chrome[^40]. El orquestador contrarresta esta discrepancia inicializando un perfil de configuración que deshabilita la extensión interna mediante el argumento de línea de comandos --disable-brave-extension.

En navegadores como Opera y Opera GX bajo entornos Windows, la invocación del ejecutable primario delega el control en una utilidad desacoplada denominada launcher.exe, la cual finaliza inmediatamente tras delegar la ejecución gráfica en procesos hijos huérfanos[^38]. El monitor del orquestador mitiga este desacoplamiento evitando la supervisión exclusiva del identificador de proceso (PID) inicial y adoptando un sondeo directo sobre la interfaz HTTP local /json/version para determinar cuándo la instancia de renderizado ha completado su inicialización.

Para Vivaldi y Arc, cuyas interfaces de usuario sobrecargan el proceso de renderizado con barras de herramientas personalizadas y docks flotantes, se aplica una restricción estricta mediante --headless=new y --hide-scrollbars, garantizando que únicamente se inicialice el lienzo web y se omita cualquier componente de la interfaz de la aplicación[^2].

Finalmente, en navegadores enfocados en privacidad estricta como LibreWolf, se desactivan las protecciones nativas contra la generación de huellas digitales (anti-fingerprinting y letterboxing) modificando las preferencias en el archivo user.js inyectado en el perfil temporal, evitando así la distorsión deliberada de métricas de pantalla y la inyección de ruido en elementos \<canvas>.

## Gestión del Ciclo de Vida de Procesos, Aislamiento y Prevención de Procesos Huérfanos

La ejecución paralela y automatizada de navegadores requiere garantizar el aislamiento riguroso del estado del sistema y la recolección determinista de todos los recursos del sistema operativo al concluir las capturas[^3].

El orquestador genera perfiles temporales únicos y efímeros para cada invocación, ubicándolos en los directorios volátiles del sistema (%TEMP%\omnisnap_\<uuid> en Windows o /tmp/omnisnap_\<uuid> en macOS y Linux)[^3]. Para los derivados de Blink, la directiva --user-data-dir se combina con banderas de aislamiento que desactivan servicios secundarios del navegador: --no-first-run, --no-default-browser-check, --disable-sync, --disable-background-networking, --disable-component-update y --metrics-recording-only[^3].

Para los navegadores basados en Gecko, se escribe dinámicamente un archivo de configuración mínima user.js dentro del directorio temporal asignado mediante --profile, habilitando el agente remoto y suprimiendo las interfaces de introducción y recolección de telemetría mediante las directivas user_pref("remote.active-protocols", 3);, user_pref("devtools.debugger.remote-enabled", true); y user_pref("toolkit.telemetry.reportingpolicy.firstRun", false);[^3]. Tras la desconexión del protocolo, el orquestador purga de manera recursiva estos directorios del sistema de archivos.

La asignación de puertos de comunicación loopback se resuelve de manera dinámica para evitar colisiones entre procesos paralelos o interferencias con instancias activas del usuario[^3]. En los motores Blink, se pasa la bandera --remote-debugging-port=0, lo que instruye al núcleo a enlazar un puerto TCP efímero disponible provisto por la pila de red del sistema operativo[^7]. El orquestador intercepta el descriptor de error estándar (stderr) del subproceso, procesa la expresión regular DevTools listening on ws://127.0.0.1:(\d+) y establece la conexión WebSocket contra la dirección resuelta.

En los motores Gecko, Firefox soporta el paso de un puerto dinámico mediante --remote-debugging-port \<port>[^7]. El orquestador ejecuta una reserva previa abriendo un socket TCP efímero en 127.0.0.1:0, extrae el número de puerto asignado, cierra dicho socket y lanza el binario de Firefox vinculado a dicho puerto de manera inmediata, eliminando el riesgo de carreras de red y exponiendo la interfaz BiDi en ws://127.0.0.1:\<puerto>/session[^7].

Para la finalización de procesos, los métodos estándar de terminación basados en el envío de señales al proceso raíz resultan insuficientes, ya que los navegadores modernos emplean topologías multiproceso que separan los procesos de red, GPU y aislamiento de pestañas en árboles independientes[^41].

En entornos Windows, el orquestador utiliza la API Win32 de Job Objects[^45]. Antes de generar el subproceso del navegador, se inicializa un objeto de trabajo invocando CreateJobObject, se define la estructura JOBOBJECT_EXTENDED_LIMIT_INFORMATION configurada con el flag JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, y se asocia el manejador del proceso recién creado mediante AssignProcessToJobObject[^45]. Cuando el orquestador cierra el manejador del Job Object, o si el proceso principal finaliza inesperadamente, el núcleo de Windows destruye de manera atómica todos los procesos descendientes asociados[^45].

En entornos Linux y macOS, los procesos del navegador se crean en una sesión desacoplada asignando un nuevo grupo de procesos mediante la directiva detached: true en el entorno de Node.js o preexec_fn=os.setsid en Python[^41]. Al concluir la captura, el orquestador envía inicialmente la señal SIGTERM al identificador de grupo negativo (-pid), otorga un tiempo de espera de quinientos milisegundos para permitir la desasignación ordenada de descriptores de archivos, y procede con SIGKILL si algún subproceso permanece en memoria[^41].

El modelo de concurrencia del orquestador gestiona la ejecución de manera asimétrica para ajustarse a las limitaciones de los controladores del sistema. Los navegadores pertenecientes a las familias Blink y Gecko se ejecutan en un pool de trabajadores asíncronos concurrentes que procesa tantas tareas en paralelo como núcleos lógicos de procesamiento se encuentren disponibles[^14].

Por el contrario, la automatización de Safari a través de /usr/bin/safaridriver en macOS impone una restricción por diseño: el sistema operativo no permite más de una sesión activa simultánea de WebDriver[^17]. Múltiples instancias concurrentes provocan el bloqueo del puerto HTTP o el rechazo de la conexión[^15]. Para gestionar esta limitación, el despachador de tareas segrega las ejecuciones de Safari en una cola de procesamiento serial estricto con estructura FIFO, garantizando que cada prueba sobre WebKit adquiera, utilice y libere la sesión del sistema operativo de manera aislada antes de que el siguiente elemento de la cola sea despachado[^17].

| **Sistema Operativo** | **Familia Técnica** | **Mecanismo de Aislamiento de Estado** | **Asignación de Puerto Loopback** | **Estrategia de Terminación de Procesos** | **Modelo de Concurrencia** |
|---|---|---|---|---|---|
| **Windows** | Blink (Chrome, Edge, Brave, etc.) | Carpeta temporal en %TEMP% vía --user-data-dir[^3] | Asignación dinámica del SO con --remote-debugging-port=0[^7] | Win32 Job Object con JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE[^45] | Paralelo (Pool asíncrono no bloqueante) |
| **Windows** | Gecko (Firefox, Floorp, etc.) | Carpeta temporal en %TEMP% vía --profile con user.js inyectado[^3] | Socket efímero previo vinculado a --remote-debugging-port \<puerto>[^7] | Win32 Job Object con JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE[^45] | Paralelo (Pool asíncrono no bloqueante) |
| **macOS** | Blink | Carpeta temporal en /tmp vía --user-data-dir | Asignación dinámica del SO con --remote-debugging-port=0[^7] | Despacho de SIGTERM y SIGKILL a Process Group (-pid)[^41] | Paralelo (Pool asíncrono no bloqueante) |
| **macOS** | Gecko | Carpeta temporal en /tmp vía --profile con user.js inyectado[^3] | Socket efímero previo vinculado a --remote-debugging-port \<puerto>[^7] | Despacho de SIGTERM y SIGKILL a Process Group (-pid)[^41] | Paralelo (Pool asíncrono no bloqueante) |
| **macOS** | WebKit (Safari nativo) | Ventana de automatización aislada por el controlador del sistema[^17] | Puerto local parametrizado mediante safaridriver -p \<puerto>[^19] | Petición DELETE /session/{id} y terminación controlada del driver[^18] | Serial Estricto (Cola FIFO de una sola sesión)[^17] |
| **Linux** | Blink | Carpeta temporal en /tmp vía --user-data-dir | Asignación dinámica del SO con --remote-debugging-port=0[^7] | Despacho de SIGTERM y SIGKILL a Process Group (-pid)[^41] | Paralelo (Pool asíncrono no bloqueante) |
| **Linux** | Gecko | Carpeta temporal en /tmp vía --profile con user.js inyectado[^3] | Socket efímero previo vinculado a --remote-debugging-port \<puerto>[^7] | Despacho de SIGTERM y SIGKILL a Process Group (-pid)[^41] | Paralelo (Pool asíncrono no bloqueante) |
| **Linux** | WebKitGTK (Epiphany) | Perfil temporal en /tmp administrado por el controlador[^9] | Puerto local parametrizado mediante WebKitWebDriver -p \<puerto>[^9] | Petición DELETE /session/{id} y terminación de proceso[^18] | Paralelo (Sujeto a instancias de WebKitWebDriver)[^9] |

## Controladores de Protocolo Nativo y Mecánicas de Captura

Cada familia de navegadores expone capacidades heterogéneas para la representación y captura de páginas web de longitud completa[^2]. El orquestador unifica estas particularidades implementando adaptadores de bajo nivel que generan salidas gráficas consistentes sin depender de intermediarios pesados[^5].

En los motores de la familia Blink, la comunicación opera mediante un cliente WebSocket conectado a la URL extraída del sondeo inicial[^7]. El adaptador inicializa la sesión habilitando el dominio de página mediante el comando Page.enable y fuerza las métricas del dispositivo a través de Emulation.setDeviceMetricsOverride, fijando el ancho y alto del viewport junto con un factor de escala de píxeles (deviceScaleFactor) idéntico a uno[^2].

Tras invocar Page.navigate y esperar la resolución de estabilidad, el controlador solicita las dimensiones totales de la maquetación despachando Page.getLayoutMetrics[^8]. La estructura devuelta contiene la propiedad cssContentSize, que define el ancho y alto absoluto del contenido desplazable[^8]. Seguidamente, se emite el comando de captura especificando la geometría completa del documento:

```json
{
  "id": 10,
  "method": "Page.captureScreenshot",
  "params": {
    "format": "png",
    "captureBeyondViewport": true,
    "fromSurface": true,
    "clip": {
      "x": 0,
      "y": 0,
      "width": cssContentSize.width,
      "height": cssContentSize.height,
      "scale": 1
    }
  }
}
```

La directiva captureBeyondViewport: true instruye al motor gráfico Skia a dimensionar la superficie de renderizado a la totalidad del lienzo sin desplazar el scroll del usuario ni alterar la posición de los contenedores estáticos[^8]. Este enfoque de pasada única elimina la repetición de encabezados pegajosos (sticky headers) característica de los métodos basados en scroll iterativo[^37]. En documentos cuya altura sobrepasa el límite de textura de la GPU (típicamente 16.384 píxeles), el controlador segmenta la captura en bloques de 8.000 píxeles manteniendo la compensación del clip y fusionando los fragmentos en un único búfer de imagen en memoria[^11].

Para la familia Gecko, Firefox provee una implementación de W3C WebDriver BiDi a través de su interfaz WebSocket interna[^6]. El adaptador inicializa la sesión mediante el comando session.new con un objeto de capacidades vacío, recupera el contexto de navegación activo a través de browsingContext.getTree y normaliza el tamaño y la densidad visual invocando browsingContext.setViewport con devicePixelRatio: 1.0[^7].

La navegación se ejecuta mediante browsingContext.navigate configurando el parámetro "wait": "complete", lo que suspende la respuesta hasta que el documento y sus recursos esenciales hayan concluido su carga[^55]. La captura de página completa se resuelve a nivel de protocolo sin requerir cálculos de dimensiones de la ventana gráfica[^5]:

```json
{
  "id": 11,
  "method": "browsingContext.captureScreenshot",
  "params": {
    "context": "c62f2732-4217-498c-859a-115858cfd7be",
    "origin": "document",
    "format": { "type": "image/png" }
  }
}
```

Al asignar "origin": "document", el compositor WebRender dimensiona internamente el búfer de imagen a las coordenadas totales del documento desplazable, generando una captura continua que incluye todo el contenido situado fuera del pliegue de la pantalla[^5].

En la familia WebKit (Safari en macOS y WebKitGTK en Linux), la especificación formal de W3C WebDriver Classic delimita el endpoint GET /session/{id}/screenshot exclusivamente a la porción visible del viewport[^2]. Cualquier intento de forzar la ventana a dimensiones mayores al monitor físico mediante llamadas a Set Window Rect queda limitado por las dimensiones del escritorio del sistema operativo[^2].

Para sortear esta limitación, el orquestador implementa un pipeline de desplazamiento y cosido (scroll-and-stitch) algorítmico asistido[^2]. El controlador inyecta un script sincronizado mediante POST /session/{id}/execute/sync que inspecciona document.documentElement.scrollHeight, la altura interna window.innerHeight y la escala de visualización window.devicePixelRatio[^22]. Previo al desplazamiento, el script analiza el DOM en búsqueda de elementos cuyas propiedades de estilo computadas contengan position: fixed o position: sticky[^22]. Tras la captura del primer encuadre superior, el script establece temporalmente element.style.visibility = 'hidden' sobre dichos elementos fijos, impidiendo que se dibujen repetidamente en cada sección vertical sucesiva[^22].

El orquestador itera desplazando la ventana mediante window.scrollTo(0, offsetY), aguarda la sincronización del compositor gráfico a través de dos llamadas encadenadas a requestAnimationFrame, y captura la porción del viewport[^22]. Al alcanzar el final del documento, los elementos con estilos alterados recuperan su visibilidad original y los fragmentos gráficos se concatenan verticalmente en un lienzo único recortando las regiones solapadas mediante la librería de manipulación de imágenes nativa (Pillow en Python o Sharp en Node.js)[^2].

| **Familia Técnica** | **Comando de Normalización de Viewport** | **Comando de Captura Primario** | **Parámetro de Lienzo Extendido** | **Mitigación de Capas Estáticas / Sticky** |
|---|---|---|---|---|
| **Blink** | Emulation.setDeviceMetricsOverride (deviceScaleFactor: 1)[^2] | Page.captureScreenshot[^8] | captureBeyondViewport: true, con caja obtenida de Page.getLayoutMetrics[^8] | Nativo: Skia rasteriza el lienzo global en una pasada sin alterar el scroll del usuario[^11] |
| **Gecko** | browsingContext.setViewport (devicePixelRatio: 1.0)[^52] | browsingContext.captureScreenshot[^5] | origin: "document"[^5] | Nativo: WebRender dimensiona el búfer a las coordenadas de la raíz del documento[^5] |
| **WebKit** | Normalización de escala por recorte y remuestreo según window.devicePixelRatio[^60] | GET /session/{id}/screenshot iterativo[^18] | Algoritmo de scroll iterativo sobre window.scrollTo con solape dinámico[^22] | Inyección de script: ocultamiento temporal de nodos con position: fixed/sticky tras el primer cuadro[^22] |

## Heurísticas de Estabilización Determinista de Renderizado

Uno de los principales factores de fallo en las herramientas de verificación visual radica en la captura de pantallas sobre estados transitorios del navegador[^2]. La práctica de establecer retardos arbitrarios de tiempo mediante funciones de espera fija introduce demoras innecesarias o produce fallos de sincronización ante variaciones en la carga de CPU o latencia de red[^2].

El orquestador resuelve este problema descartando retardos estáticos y ejecutando una rutina heurística inyectada en el contexto de navegación que evalúa la estabilidad del documento en múltiples fases secuenciales antes de autorizar la captura[^63].

La fase inicial verifica que la estructura base del documento se encuentre disponible validando que document.readyState haya alcanzado el valor 'complete'[^55]. Si la navegación se encuentra en progreso, la rutina se suscribe al evento de carga (window.addEventListener('load')) antes de continuar con las validaciones subsiguientes[^55].

La segunda fase garantiza la convergencia del tráfico de red. Para mitigar peticiones asíncronas lentas que condicionan la visualización de datos, un script base parchea window.fetch y XMLHttpRequest.prototype.send, manteniendo un registro de las solicitudes activas. La rutina exige que el número de peticiones de red en vuelo permanezca en cero durante un periodo de silencio ininterrumpido no menor a quinientos milisegundos.

La tercera fase aborda la tipografía web para evitar el parpadeo de texto sin estilo (Flash of Unstyled Text o FOUT) o texto invisible (Flash of Invisible Text o FOIT), anomalías que alteran drásticamente la altura de las cajas de texto y provocan discrepancias falsas de maquetación[^10]. La rutina evalúa la promesa provista por la API estándar de fuentes del navegador:

```javascript
if (document.fonts && document.fonts.ready) {
  await document.fonts.ready;
}
```

Esta instrucción detiene el flujo hasta que todas las fuentes externas definidas en las reglas @font-face hayan finalizado su descarga, descompresión y registro en el subsistema de fuentes del sistema operativo[^63].

La cuarta fase neutraliza el movimiento provocado por transiciones CSS y animaciones JavaScript activas que modificarían la posición de los componentes entre una captura y otra[^63]. Mediante la API estándar Web Animations, el script localiza todos los objetos de animación mediante document.getAnimations()[^63]. Aquellas instancias que presentan un estado de ejecución activo y poseen una duración finita son pausadas o forzadas algorítmicamente hacia su tiempo de finalización (animation.finish()), asegurando que barras de desplazamiento, carruseles o menús desplegables se consoliden en su geometría definitiva[^22].

La quinta fase ejecuta un ciclo de pre-activación de contenido diferido (lazy loading)[^22]. Muchos entornos modernos vinculan la descarga y renderizado de imágenes a la visibilidad de los elementos a través de la API IntersectionObserver[^37]. Si el orquestador capturase la página sin desplazar el documento, las imágenes ubicadas por debajo del pliegue de la pantalla se obtendrían como marcadores de posición vacíos[^22]. La sonda ejecuta un desplazamiento vertical rápido hacia el final del documento mediante window.scrollTo(0, document.body.scrollHeight) y retorna de inmediato a la coordenada superior (0, 0), forzando al navegador a despachar las solicitudes de imagen y decodificar los gráficos correspondientes[^22].

La sexta y última fase vacía la cola de trabajo del compositor gráfico del navegador mediante la sincronización de dos ciclos sucesivos de refresco de pantalla estructurados de la siguiente forma:

```javascript
await new Promise(resolve => {
  requestAnimationFrame(() => {
    requestAnimationFrame(resolve);
  });
});
```

El primer ciclo de requestAnimationFrame asegura que todas las mutaciones estructurales del DOM, recalculaciones de estilos y ejecuciones de scripts hayan sido procesadas por el motor de renderizado[^62]. El segundo ciclo garantiza que el árbol de renderizado haya sido transferido formalmente a la GPU y que los búferes de visualización reflejen el estado definitivo de los píxeles[^62].

## Detección de Anomalías DOM y Algoritmos de Comparación Perceptual

Para transformar capturas de pantalla de navegadores en información de depuración estructurada, el orquestador opera en dos niveles complementarios: una auditoría estática inyectada en el árbol DOM y un motor de análisis de diferencias visuales perceptuales[^40].

### Auditoría Estructural de Anomalías de Maquetación

Antes de transferir la imagen a disco, un analizador inyectado examina la geometría física del documento computando violaciones objetivas de diseño[^70]:

- **Desbordamiento Raíz Horizontal Involuntario:** El analizador compara document.documentElement.scrollWidth frente a document.documentElement.clientWidth[^68]. Si el ancho desplazable supera el ancho del cliente, se itera sobre los nodos visibles evaluando sus coordenadas físicas mediante getBoundingClientRect() para identificar y reportar aquellos elementos específicos que exceden el ancho del viewport y provocan la aparición de una barra de scroll horizontal indeseada[^68].
- **Corte Involuntario de Contenido (Clipping Faults):** Se examinan aquellos elementos que contienen texto o controles interactivos y cuyos contenedores inmediatos declaran propiedades CSS overflow: hidden o overflow: clip[^40]. La rutina calcula si la caja delimitadora del elemento hijo excede los límites calculados del contenedor padre, marcando como anomalía el texto truncado fuera del área de visualización[^40].
- **Colisión Accidental de Elementos Hermanos:** Se analizan los elementos que comparten un mismo contexto de apilamiento visual (stacking context) y nivel de elevación (z-index)[^40]. Empleando el algoritmo de intersección de Cajas Delimitadoras Alineadas con los Ejes (AABB - Axis-Aligned Bounding Box), el script verifica si dos nodos sin relación jerárquica colisionan físicamente en sus coordenadas de pantalla[^40]:

$$
\text{Colisión}(A, B) = (A_{\text{left}} < B_{\text{right}}) \land (A_{\text{right}} > B_{\text{left}}) \land (A_{\text{top}} < B_{\text{bottom}}) \land (A_{\text{bottom}} > B_{\text{top}})
$$

Si la ecuación resulta verdadera y los estilos computados demuestran que ambos nodos poseen contenido visible y opacidad superior a cero, la intersección se cataloga como una colisión accidental que dificulta la legibilidad o interacción del usuario[^40].

- **Recursos Gráficos Incompletos:** Se realiza una consulta transversal sobre todas las etiquetas \<img> y \<svg> presentes en el DOM visible, verificando que su propiedad naturalWidth o sus dimensiones internas no sean iguales a cero, lo cual denota fallos de red en la carga del recurso o enlaces rotos[^69].

### Motor de Comparación Perceptual Inmune al Suavizado de Fuentes

La comparación tradicional de imágenes basada en igualdad estricta de valores RGB fracasa sistemáticamente en pruebas cruzadas entre navegadores[^10]. Cada sistema operativo implementa tecnologías propietarias de renderizado tipográfico: ClearType y DirectWrite en Windows alteran la luminancia de subpíxeles horizontalmente, CoreText en macOS enfatiza el grosor tipográfico preservando la fidelidad del glifo, y FreeType en Linux prioriza la alineación estricta con la cuadrícula de píxeles[^10]. Comparar capturas de un mismo sitio web entre Chrome sobre Windows y Firefox sobre Linux mediante un diff de píxeles estricto marcaría la totalidad del texto como un error visual masivo, ocultando discrepancias funcionales genuinas[^10].

El orquestador resuelve este problema mediante un pipeline perceptual que procesa las imágenes de forma desacoplada del suavizado de fuentes[^69]. En primer lugar, se establece un navegador de referencia (predeterminado a Google Chrome debido a su cuota de mercado con el motor Blink). Las dimensiones de los lienzos a comparar se igualan mediante un recorte geométrico alineado a la esquina superior izquierda (0, 0).

Seguidamente, se evalúa el Índice de Similitud Estructural (SSIM), el cual analiza las correlaciones espaciales en ventanas locales gaussianas de $8 \times 8$ píxeles, evaluando las pérdidas de luminancia, contraste y estructura espacial en lugar de diferencias absolutas de color[^75]:

$$
\text{SSIM}(x, y) = \frac{(2\mu_x\mu_y + C_1)(2\sigma_{xy} + C_2)}{(\mu_x^2 + \mu_y^2 + C_1)(\sigma_x^2 + \sigma_y^2 + C_2)}
$$

Donde $\mu_x$ y $\mu_y$ representan las medias locales de luminosidad, $\sigma_x^2$ y $\sigma_y^2$ las varianzas de contraste, $\sigma_{xy}$ la covarianza cruzada entre ambas imágenes, y $C_1 = (K_1 L)^2$ junto con $C_2 = (K_2 L)^2$ actúan como constantes de regularización respecto al rango dinámico $L = 255$, con valores estandarizados $K_1 = 0.01$ y $K_2 = 0.03$[^78].

La divergencia visual global se calcula como la Disimilitud Estructural (DSSIM)[^76]:

$$
\text{DSSIM}(x, y) = \frac{1 - \text{SSIM}(x, y)}{2}
$$

Para aislar y descartar de manera definitiva el ruido provocado por el suavizado tipográfico perimetral, el motor realiza una transformación del espacio de color RGB al espacio perceptual uniforme CIELAB ($L^*, a^*, b^*$) y calcula la distancia de color perceptual euclidiana o $\Delta E_{00}$ sobre cada píxel sospechoso[^76].

Si un píxel presenta discrepancias pero se localiza a una distancia euclidiana inferior a 1,5 píxeles de un borde de alto contraste y su magnitud cromática no excede el umbral de discriminación visual humano ($\Delta E < 2.3$), el sistema clasifica el punto como un artefacto normal de antialiasing y lo excluye de la máscara de errores[^79].

Los puntos restantes que superan el umbral establecido constituyen desviaciones reales de maquetación (tales como saltos de línea imprevistos, discrepancias en el cálculo de Flexbox o Grid, o márgenes desalineados) y se proyectan sobre un mapa de calor visual (heatmap) resaltado en color magenta sobre el lienzo original[^40].

| **Categoría de Defecto** | **Método de Detección** | **Métrica Matemática / Lógica** | **Umbral de Aceptación** | **Nivel de Severidad** |
|---|---|---|---|---|
| **Desbordamiento Horizontal Raíz** | Sonda inyectada en DOM | document.documentElement.scrollWidth > clientWidth[^68] | Diferencia $\leq 0\,\text{px}$ | Crítico (rotura de diseño responsivo) |
| **Corte de Texto / Botón** | Sonda inyectada en DOM | Bounding box del hijo fuera del contenedor con overflow: hidden[^40] | Intersección $< 100\%$ del área del hijo | Alto (pérdida de accesibilidad / lectura) |
| **Colisión de Elementos (AABB)** | Sonda inyectada en DOM | Ecuación de solape físico sobre nodos no emparentados[^40] | Área de intersección $= 0\,\text{px}^2$ | Alto (elementos solapados en pantalla) |
| **Recursos Faltantes / Caídos** | Sonda inyectada en DOM | img.naturalWidth === 0 o error de recurso activo[^69] | Conteo $= 0$ elementos fallidos | Crítico (imágenes rotas o iconos ausentes) |
| **Variación Estructural Visual** | Análisis de Imagen SSIM | Índice local de similitud estructural ponderado[^75] | $\text{SSIM} \geq 0.98$ entre motores distintos | Variable según tolerancia de configuración |
| **Disimilitud Perceptual** | Análisis de Imagen DSSIM | Métrica normalizada de distancia estructural[^76] | $\text{DSSIM} \leq 0.01$ en la misma familia de motor | Alto si supera el umbral configurado |
| **Ruido por Antialiasing** | Filtro Perceptual CIELAB | Diferencia cromática $\Delta E_{00}$ en radio perimetral $\leq 1.5\,\text{px}$[^76] | $\Delta E < 2.3$ (JND - Just Noticeable Difference) | Informativo (filtrado automático del diff) |

## Requisitos de Producto y Matriz Operativa

La especificación del producto define el comportamiento operativo del orquestador mediante requisitos funcionales y no funcionales estructurados para su validación formal.

| **Identificador** | **Requisito Funcional** | **Mecanismo de Implementación** | **Criterio de Aceptación y Verificación** |
|---|---|---|---|
| **FR-01** | Descubrimiento Automático Universal | Escaneo asíncrono de registro en Windows, Spotlight en macOS y XDG/Flatpak/Snap en Linux[^24] | Retorna el catálogo completo de binarios instalados en $< 300\,\text{ms}$ sin configuración manual. |
| **FR-02** | Empaquetado Autónomo Ligero | Despliegue en npm y PyPI sin dependencias binarias externas ni navegadores preempaquetados | El tamaño final de descarga del paquete debe permanecer estrictamente por debajo de los $15\,\text{MB}$. |
| **FR-03** | Aislamiento Absoluto de Sesión | Generación de perfiles efímeros en carpetas temporales volátiles del sistema operativo[^3] | Al concluir la prueba o producirse un fallo, el directorio temporal se destruye sin dejar residuos[^3]. |
| **FR-04** | Protocolos Nativos sin Proxies | Clientes de red directos hacia CDP sobre Blink, WebDriver BiDi sobre Gecko y W3C REST sobre WebKit[^5] | Ningún proceso intermediario como chromedriver o geckodriver debe ser invocado o instalado[^8]. |
| **FR-05** | Captura Determinista de Documento | Rasterizado de una sola pasada en Blink/Gecko y scroll-and-stitch algorítmico en WebKit[^5] | Generación de imágenes PNG de página completa preservando encabezados fijos y capas pegajosas[^22]. |
| **FR-06** | Estabilización Heurística Previa | Sincronización inyectada evaluando estado de red, fuentes tipográficas, animaciones y compositor[^63] | Cero capturas sobre estados incompletos, fuentes sin renderizar o animaciones en progreso[^62]. |
| **FR-07** | Auditoría Estructural del DOM | Análisis AABB para colisiones, detección de desbordamientos de scroll y verificación de imágenes[^40] | Listado estructurado con selectores CSS precisos y coordenadas de cada anomalía geométrica[^70]. |
| **FR-08** | Diff Perceptual Cross-Browser | Algoritmos SSIM y DSSIM complementados con filtrado de antialiasing en espacio de color CIELAB[^75] | Emisión de mapas de calor visuales que descartan diferencias de rasterizado tipográfico[^69]. |

| **Identificador** | **Requisito No Funcional** | **Parámetro Operativo** | **Estrategia de Mitigación y Cumplimiento** |
|---|---|---|---|
| **NFR-01** | Rendimiento y Concurrencia | Ejecución simultánea en Blink y Gecko escalada al número de núcleos lógicos del host[^14] | Pool asíncrono con asignación dinámica de puertos loopback efímeros[^7]. |
| **NFR-02** | Manejo de Restricciones del Sistema | Ejecución serial estricta en macOS Safari impuesta por las políticas de /usr/bin/safaridriver[^17] | Cola FIFO exclusiva para tareas dirigidas a WebKit, evitando colisiones de sesión[^17]. |
| **NFR-03** | Eliminación de Procesos Huérfanos | Cero procesos de renderizado o de red remanentes en memoria tras la terminación de la suite[^41] | Uso mandatorio de Windows Job Objects y grupos de procesos POSIX con señales escalonadas[^41]. |
| **NFR-04** | Portabilidad de Entornos | Funcionamiento idéntico en Windows 10/11, macOS 12+ y distribuciones Linux modernas | Abstracción de capas del sistema operativo mediante adaptadores de plataforma nativos. |
| **NFR-05** | Resiliencia y Tolerancia a Fallos | La falla inesperada o ausencia de un navegador no debe interrumpir el lote de pruebas | Degradación elegante: registro de error en metadatos y continuidad de los navegadores restantes. |
| **NFR-06** | Compatibilidad con Entornos CI/CD | Operación autónoma en contenedores sin interfaz gráfica física (GitHub Actions, GitLab CI) | Forzado de --headless=new en Blink y --headless en Gecko con emulación de display virtual[^2]. |

La experiencia operativa se articula en torno a una interfaz de línea de comandos unificada que permite tanto el diagnóstico rápido como la ejecución masiva de pruebas de regresión visual:

```bash
# Modo 1: Diagnóstico de navegadores instalados en el equipo anfitrión
omnisnap detect

# Modo 2: Captura rápida con cero configuración y valores por defecto inteligentes
omnisnap https://app.example.com --full-page --output ./visual-report

# Modo 3: Auditoría exhaustiva cross-browser con matriz perceptual y reporte DOM
omnisnap https://app.example.com \
  --browsers chrome,brave,firefox,safari \
  --viewports 1920x1080,390x844 \
  --diff-threshold 0.05 \
  --run-dom-audit \
  --format html,json
```

El resultado del procesamiento se estructura en un archivo estandarizado summary.json que expone los metadatos de ejecución, métricas perceptuales y anomalías detectadas:

```json
{
  "$schema": "https://omnisnap.dev/schemas/v1/summary.json",
  "metadata": {
    "targetUrl": "https://app.example.com",
    "timestamp": "2026-11-06T21:07:00Z",
    "hostPlatform": {
      "os": "darwin",
      "architecture": "arm64",
      "osVersion": "15.1"
    },
    "viewport": {
      "width": 1920,
      "height": 1080,
      "devicePixelRatio": 1.0
    }
  },
  "runs": [
    {
      "browserName": "Google Chrome",
      "engine": "Blink",
      "version": "134.0.6998.35",
      "executablePath": "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
      "status": "success",
      "executionTimeMs": 1420,
      "artifacts": {
        "screenshot": "captures/chrome_1920x1080.png"
      },
      "domAudit": {
        "horizontalOverflow": false,
        "brokenImagesCount": 0,
        "clippingFaults": [],
        "elementCollisions": []
      }
    },
    {
      "browserName": "Mozilla Firefox",
      "engine": "Gecko",
      "version": "136.0",
      "executablePath": "/Applications/Firefox.app/Contents/MacOS/firefox",
      "status": "success",
      "executionTimeMs": 1850,
      "artifacts": {
        "screenshot": "captures/firefox_1920x1080.png"
      },
      "domAudit": {
        "horizontalOverflow": true,
        "brokenImagesCount": 0,
        "clippingFaults": [
          {
            "selector": "div.card-body > p.description",
            "reason": "Text clipped by parent overflow: hidden"
          }
        ],
        "elementCollisions": []
      }
    }
  ],
  "comparisons": [
    {
      "baseline": "Google Chrome",
      "target": "Mozilla Firefox",
      "metrics": {
        "ssim": 0.9821,
        "dssim": 0.00895,
        "divergentPixels": 1240,
        "thresholdExceeded": false
      },
      "diffArtifact": "diffs/chrome_vs_firefox_diff.png"
    }
  ]
}
```

Junto con el archivo estructurado de metadatos, la suite genera un panel interactivo autónomo compilado en report.html. Este reporte opera sin dependencias remotas y puede visualizarse abriendo el archivo local directamente en cualquier navegador.

La interfaz provee tres modalidades analíticas de inspección: una vista en matriz que sincroniza el desplazamiento y aumento visual de todas las capturas de forma acoplada; una vista de cortina divisoria (split slider) que permite contrastar directamente el navegador base contra un motor secundario arrastrando un control vertical para verificar desajustes milimétricos; y una vista de máscara de calor que proyecta las áreas donde la discrepancia estructural sobrepasa el umbral de tolerancia fijado.

En la parte inferior de la visualización, un panel interactivo colapsable detalla los hallazgos geométricos obtenidos durante la auditoría del DOM, permitiendo al desarrollador hacer clic sobre cualquier anomalía (como desbordamiento o colisión) para enfocar automáticamente las coordenadas afectadas sobre las capturas correspondientes.

## Conclusiones y Recomendaciones de Implementación

La formulación técnica expuesta demuestra la viabilidad de implementar un orquestador visual ligero, multiplataforma y sin configuración técnica previa, sustituyendo las complejas suites tradicionales por interacciones de red directas contra los protocolos nativos de depuración[^5].

El desarrollo debe priorizar la solidez del módulo de descubrimiento de binarios en el sistema operativo anfitrión. En plataformas Windows, la combinación de consultas en el registro bajo la clave Clients\StartMenuInternet junto con la verificación de rutas de usuario en App Paths resuelve el catálogo completo de navegadores comerciales instalados[^24]. En sistemas macOS y Linux, la delegación en la utilidad Spotlight (mdfind) y el procesamiento ordenado de entradas de escritorio XDG, paquetes Flatpak y paquetes Snap proporcionan una cobertura exhaustiva sin requerir intervención por parte del usuario[^33].

La interacción directa mediante conexiones WebSocket implementando Chrome DevTools Protocol para la familia Blink y el estándar emergente W3C WebDriver BiDi para la familia Gecko proporciona un control determinista de alta velocidad y elimina la necesidad de distribuir ejecutables controladores pesados[^5]. Para el soporte de WebKit en macOS, la arquitectura debe incorporar un despachador asimétrico que procese las solicitudes serialmente debido al bloqueo inherente a nivel de sistema operativo de safaridriver, integrando un algoritmo coordinado de scroll-and-stitch con supresión temporal de capas con posicionamiento fijo[^17].

La fiabilidad operativa y la erradicación de falsos positivos en las pruebas dependen de dos subsistemas críticos: la aplicación de rutinas heurísticas inyectadas que garanticen la estabilidad del DOM, la decodificación de tipografías web y la finalización de microtareas del compositor antes de autorizar el rasterizado; y la implementación de un pipeline de comparación perceptual basado en SSIM, DSSIM y espacios de color CIELAB que neutralice las variaciones inherentes al suavizado de fuentes entre sistemas operativos[^62]. Esta estrategia asegura la detección precisa de anomalías visuales y estructurales con un consumo mínimo de recursos y una integración sin fricciones en cualquier entorno de desarrollo o integración continua.

#### Obras citadas

[^1]: Selenium WebDriver: JavaScript Screenshot Guide - TestMu AI, https://www.testmuai.com/blog/taking-screenshots-in-selenium-webdriver-using-javascript/
[^2]: Selenium Screenshot Guide: Code Examples (2026), https://www.screensnap.pro/blog/selenium-screenshot-guide
[^3]: Firefox Remote Debugging - Learn SecByte, https://learn.secbyte.org/tools/firefox-remote-debugging
[^4]: getting a list of installed browsers on the computer - Stack Overflow, https://stackoverflow.com/questions/43633190/getting-a-list-of-installed-browsers-on-the-computer
[^5]: browsingContext.captureScreenshot command - WebDriver | MDN, https://developer.mozilla.org/en-US/docs/Web/WebDriver/Reference/BiDi/Modules/browsingContext/captureScreenshot
[^6]: WebDriver BiDi - W3C, https://www.w3.org/TR/webdriver-bidi/
[^7]: Create a WebDriver BiDi connection - MDN Web Docs, https://developer.mozilla.org/en-US/docs/Web/WebDriver/How_to/Create_BiDi_connection
[^8]: The complete guide to the Chrome DevTools Protocol - daily.dev, https://daily.dev/posts/the-complete-guide-to-the-chrome-devtools-protocol-sd4bmklke
[^9]: Advancing WebDriver BiDi support in WebKit | PDF - Slideshare, https://www.slideshare.net/slideshow/advancing-webdriver-bidi-support-in-webkit/281229125
[^10]: visual-testing - Claude Skill | AI Agent Skills, https://aiagentskills.net/skill/petrkindlmann-qa-skills-visual-testing
[^11]: Selenium Full Page Screenshot: Firefox, Chrome, Stitching, https://renderscreenshot.com/blog/selenium-full-page-screenshot
[^12]: 24.2 CDP Layer Implementation - OpenClaw-Book, https://www.openclawbook.xyz/en/ch24-browser-control/24.2-cdp-layer-implementation
[^13]: WebDriver Bidi Protocol - WebdriverIO, https://webdriver.io/docs/api/webdriverBidi
[^14]: Firefox WebDriver Newsletter 136, https://fxdx.dev/firefox-webdriver-newsletter-136/
[^15]: Puppeteer fails to connect to WebDriver BiDi websocket from Firefox, https://github.com/puppeteer/puppeteer/issues/13057
[^16]: "browsingContext.captureScreenshot" should support full page, https://github.com/w3c/webdriver-bidi/issues/384
[^17]: About WebDriver for Safari | Apple Developer Documentation, https://developer.apple.com/documentation/webkit/about-webdriver-for-safari
[^18]: WebDriver - W3C, https://www.w3.org/TR/webdriver2/
[^19]: Test Automation on Safari Browser with Safari Driver and Selenium, https://www.testmuai.com/blog/selenium-safaridriver-macos/
[^20]: WebDriver - W3C, https://www.w3.org/TR/2015/WD-webdriver-20150808/
[^21]: README.md - agent0ai/a0-connector - GitHub, https://github.com/agent0ai/a0-connector/blob/main/README.md
[^22]: How to Take a Scrolling Screenshot (2026 Guide) - SnagitPro, https://snagitpro.com/how-to-take-a-scrolling-screenshot/
[^23]: Ability to take full page screenshot · Issue #1141 - GitHub, https://github.com/SeleniumHQ/selenium/issues/1141
[^24]: Do Not Cross The 'RedLine' Stealer: Detections and Analysis - Splunk, https://www.splunk.com/en_us/blog/security/do-not-cross-the-redline-stealer-detections-and-analysis.html
[^25]: matterport-dl.py - GitHub, https://github.com/rebane2001/matterport-dl/blob/main/matterport-dl.py
[^26]: VirusTotal - File - VirusTotal, https://www.virustotal.com/gui/file/19d2a9c7f07aed304dfb40ef257f4b0b50daaa0b8c7eaeab167e19458c59e321/behavior/VirusTotal%20Jujubox
[^27]: How to check for installed browsers using C# for newbies, https://stackoverflow.com/questions/7567202/how-to-check-for-installed-browsers-using-c-sharp-for-newbies
[^28]: RedLine Stealer Malware Analysis Report | PDF - Scribd, https://www.scribd.com/document/758548926/Report-A-Detailed-Analysis-Of-The-Red-Line-Stealer
[^29]: InstallBuilder User Guide 26, https://releases.installbuilder.com/installbuilder/docs/installbuilder-userguide.html
[^30]: Executing command line scripts from the Oracle Forms client, https://technology.amis.nl/frontend/executing-command-line-scripts-from-the-oracle-forms-client-using-webutil/
[^31]: Batch file to find chrome installed path dynamically - Stack Overflow, https://stackoverflow.com/questions/49935377/batch-file-to-find-chrome-installed-path-dynamically
[^32]: Akira Stealer: Technical Analysis of a Modular Info-Stealing Malware, https://www.glueckkanja.com/en/posts/2025-06-16-quiet-breach
[^33]: How to Install BleachBit on Ubuntu 26.04, 24.04 and 22.04, https://linuxcapable.com/how-to-install-bleachbit-on-ubuntu-linux/
[^34]: How to Install Okular on Ubuntu 26.04, 24.04 and 22.04, https://linuxcapable.com/how-to-install-okular-on-ubuntu-linux/
[^35]: How to Install Falkon on Ubuntu 26.04, 24.04 and 22.04, https://linuxcapable.com/how-to-install-falkon-on-ubuntu-linux/
[^36]: How to Install Signal Desktop on Ubuntu 26.04, 24.04 and 22.04, https://linuxcapable.com/how-to-install-signal-on-ubuntu-linux/
[^37]: Full Page Screenshot in Edge: 3 Methods Compared, https://renderscreenshot.com/blog/full-page-screenshot-edge
[^38]: Scans still show malware on my PC - Virus, Trojan, Spyware, and, https://www.bleepingcomputer.com/forums/t/779905/scans-still-show-malware-on-my-pc/
[^39]: A Two-Stage Model for Social Network Investigations in Digital, https://www.researchgate.net/publication/344046832_A_Two-Stage_Model_for_Social_Network_Investigations_in_Digital_Forensics
[^40]: What is Overlapping Text Detection? - LlamaIndex, https://www.llamaindex.ai/glossary/overlapping-text-detection
[^41]: Killing process families with node | by Almenon - Medium, https://medium.com/@almenon214/killing-processes-with-node-772ffdd19aad
[^42]: Firefox ESR 128 --remote-debugging-port CLI option with BiDi, https://support.mozilla.org/gl/questions/1514974
[^43]: Security aspects of the Remote Agent - Firefox Source Docs - Mozilla, https://firefox-source-docs.mozilla.org/remote/Security.html
[^44]: How to kill process with entire process tree with Python on Windows?, https://stackoverflow.com/questions/70565429/how-to-kill-process-with-entire-process-tree-with-python-on-windows
[^45]: Destroying all child processes (and grandchildren) when the parent, https://devblogs.microsoft.com/oldnewthing/20131209-00/?p=2433/
[^46]: Using Windows Job Objects for Process Tree Management, https://nikhilism.com/post/2017/windows-job-objects-process-tree-management/
[^47]: How can I kill a child process run by exec on Windows?, https://stackoverflow.com/questions/43616739/how-can-i-kill-a-child-process-run-by-exec-on-windows
[^48]: true) doesn't kill entire tree if an intermediate child process is in job, https://github.com/dotnet/runtime/issues/107992
[^49]: Killing all child processes when the parent exits (Job Object), https://www.meziantou.net/killing-all-child-processes-when-the-parent-exits-job-object.htm
[^50]: TerminateProcess via os.kill on Windows? - Python Discussions, https://discuss.python.org/t/terminateprocess-via-os-kill-on-windows/30882
[^51]: The complete guide to the Chrome DevTools Protocol - Flavio Copes, https://flaviocopes.com/chrome-devtools-protocol
[^52]: Source: bidi/browsingContext.js - Selenium, https://www.selenium.dev/selenium/docs/api/javascript/bidi_browsingContext.js.html
[^53]: Commits w3c/webdriver-bidi - SemanticDiff, https://app.semanticdiff.com/gh/w3c/webdriver-bidi/commits/jrandolf/defaults?page=2
[^54]: browsingContext.getTree command - WebDriver - MDN Web Docs, https://developer.mozilla.org/en-US/docs/Web/WebDriver/Reference/BiDi/Modules/browsingContext/getTree
[^55]: browsingContext.navigate command - WebDriver - MDN Web Docs, https://developer.mozilla.org/en-US/docs/Web/WebDriver/Reference/BiDi/Modules/browsingContext/navigate
[^56]: selenium.webdriver.common.bidi.browsing_context, https://www.selenium.dev/selenium/docs/api/py/selenium_webdriver_common_bidi/selenium.webdriver.common.bidi.browsing_context.html
[^57]: selenium taking screenshots larger than expected #1067 - GitHub, https://github.com/SeleniumHQ/selenium/issues/1067
[^58]: Selenium Screenshot: 3 Ways to Capture a Web Page, https://www.screenshotrender.com/blog/selenium-screenshot
[^59]: ChromeDriver2 take screenshot is not full page [40645072], https://issues.chromium.org/40645072
[^60]: TakeFullPage screenshot using incorrect width and height (was, https://forum.katalon.com/t/takefullpage-screenshot-using-incorrect-width-and-height-was-working-properly-before/90814
[^61]: Scrolling Screenshot — Capture Full Pages Beyond the Viewport, https://screenshotedits.com/tools/scrolling-screenshot
[^62]: PDF export can differ from the webpage and may produce a blank, https://github.com/nexu-io/open-design/issues/4067
[^63]: Reliable Screenshot Capture in Headless Browsers | BotBrowser, https://botbrowser.io/en/blog/screenshot-best-practices/
[^64]: NoSQL Scalability Patterns - Kill AI, https://killai.now/archive/d93d1847-48de-4a68-b529-15962022068f?page=8
[^65]: Is there an event that fires when a web page renders its main, https://www.reddit.com/r/webdev/comments/y9tygr/is_there_an_event_that_fires_when_a_web_page/
[^66]: Streaming Performance Breakthrough, https://miasma.cpatino.com/miasma/dc138f84-b6cb-4aee-8125-512054c2e46a
[^67]: How to Take a Scrolling Screenshot on Mac: All Methods - Sowiks, https://sowiks.com/blog/scrolling-screenshot-mac
[^68]: Automated visual classification of DOM-based presentation failure, https://eprints.whiterose.ac.uk/id/eprint/165542/8/stvr.1756.pdf
[^69]: AI Tip of the Week #4: Smarter Visual Regression Testing with AI, https://shiftsync.tricentis.com/software-testing-blogs-69/ai-tip-of-the-week-4-smarter-visual-regression-testing-with-ai-2248
[^70]: (PDF) Automated visual classification of DOM‐based presentation, https://www.researchgate.net/publication/344296880_Automated_visual_classification_of_DOM-based_presentation_failure_reports_for_responsive_web_pages
[^71]: Automatic Detection of Scaling Issues for Mobile Apps - arXiv, https://arxiv.org/html/2212.04388v1
[^72]: WCAG 2.2 AA Testing: How to Test the 6 Criteria Automated, https://webspecification.com/blog/wcag-22-success-criteria/
[^73]: CodePen Games with AI - DEV Community, https://dev.to/aniruddhaadak/codepen-games-with-ai-22m6
[^74]: JavaScript - 2017-07-12 (page 7 of 20), https://chat.stackoverflow.com/transcript/17/2017/7/12/8-9
[^75]: What Is Visual Regression Testing? Definition + Code, https://renderscreenshot.com/glossary/visual-regression-testing
[^76]: Command-line Options - ImageMagick, https://imagemagick.org/command-line-options/
[^77]: FPGA implementation for real-time Chroma-key effect using Coarse, https://www.researchgate.net/publication/261120373_FPGA_implementation_for_real-time_Chroma-key_effect_using_Coarse_and_Fine_Filter
[^78]: TECHNICAL SUMMARIES - Imaging.org, https://www.imaging.org/common/uploaded%20files/pdfs/Conferences/ElectronicImaging/Abstracts/EI15-Abstracts.pdf
[^79]: ebiryu/pixeldelta: Image diff engine in Rust for visual ... - GitHub, https://github.com/ebiryu/pixeldelta
[^80]: The Fastest Image Diffing Engine You've Never Heard Of - Vizzly Blog, https://vizzly.dev/blog/honeydiff-vs-odiff-pixelmatch-benchmarks/
[^81]: geckodriver/README.md at master - GitHub, https://github.com/mozilla/geckodriver/blob/master/README.md
[^82]: How to Install Thunderbird on Ubuntu 26.04, 24.04 and 22.04, https://linuxcapable.com/how-to-install-thunderbird-on-ubuntu-linux/
