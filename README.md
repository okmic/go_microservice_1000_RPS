<h1 align="center">Go Microservice · 1000 RPS</h1>

<p>
  RESTful сервис управления кошельками на Go с использованием Gin, GORM и PostgreSQL.
  Основной фокус — высокая производительность и конкурентная безопасность.
  Все операции с балансом выполняются в транзакциях с row-level locking.
</p>

<hr />

<h2>Performance</h2>

<p>
  <b>Локальный запуск:</b>
</p>

<pre>
1000 RPS TEST
Target:     1000 req/s
Actual:     996.50 req/s
Success:    100.00%
Latency:    2.24 ms
Duration:   1.00 s
Requests:   998
Failed:     0

HIGH CONCURRENCY
Workers:    100
Requests:   1000
Success:    100.00%
RPS:        641
Duration:   1.56 s
</pre>

<p>
  <b>В Docker-контейнере:</b>
</p>

<pre>
1000 RPS TEST
Target:     1000 req/s
Actual:     694 req/s
Success:    100.00%
Latency:    1.07 ms
Duration:   1.00 s
Requests:   694

HIGH CONCURRENCY
Workers:    100
Requests:   1000
Success:    100.00%
RPS:        899
Duration:   1.11 s
</pre>

<p>
  <i>Снижение производительности в Docker связано с ограничениями CPU и сетевыми накладными расходами между контейнерами. Для максимальной производительности рекомендуется запускать приложение локально.</i>
</p>

<hr />

<h2>Quick Start</h2>

<pre><code>docker-compose up -d</code></pre>

<hr />

<h2>Tests</h2>

<p>
  Все тесты находятся в <code>tests/index_test.go</code>.
</p>

<h3>Integration tests</h3>
<ul>
  <li>Полный жизненный цикл: создание → пополнение → снятие → проверка</li>
  <li>50 конкурентных запросов к одному кошельку</li>
  <li>Обработка ошибок: 404, 400</li>
</ul>

<h3>Performance tests</h3>
<ul>
  <li>1000 RPS — 996.50 req/s, 100% успешных (локально)</li>
  <li>High concurrency — 100 воркеров, 1000 запросов</li>
  <li>Проверка целостности баланса после нагрузки</li>
</ul>

<pre><code>make test</code></pre>

<hr />

<h2>Architecture</h2>

<pre>
Client → Gin Router → Service → Repository → PostgreSQL
</pre>

<p>
  PostgreSQL <code>FOR UPDATE</code> блокирует строку перед обновлением.
  Два параллельных запроса не могут изменить один кошелек одновременно.
  Баланс никогда не становится отрицательным.
</p>

<hr />

<h2>Tech Stack</h2>

<p>
  <b>Go 1.25</b><br />
  <b>Gin</b> — HTTP роутер<br />
  <b>GORM</b> — ORM<br />
  <b>PostgreSQL 15</b> — FOR UPDATE row-level locking<br />
  <b>Docker</b> — контейнеризация<br />
  <b>Zap</b> — структурированное логирование<br />
  <b>testify</b> — тестирование
</p>