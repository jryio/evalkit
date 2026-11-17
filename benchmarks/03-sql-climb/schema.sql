CREATE TABLE customers (id INTEGER PRIMARY KEY, name TEXT, country TEXT, signed_up TEXT);
CREATE TABLE orders (id INTEGER PRIMARY KEY, customer_id INTEGER, placed_at TEXT, status TEXT, total_cents INTEGER);
CREATE TABLE refunds (order_id INTEGER, amount_cents INTEGER, refunded_at TEXT);
INSERT INTO customers VALUES
  (1,'Ada','DE','2025-11-02'),(2,'Ben','US','2025-12-14'),(3,'Chloe','DE','2026-01-20'),
  (4,'Dev','FR','2026-02-03'),(5,'Edda','US','2026-03-11'),(6,'Finn','FR','2026-04-01');
INSERT INTO orders VALUES
  (1,1,'2026-01-05','paid',12000),(2,1,'2026-03-09','paid',4550),(3,2,'2026-01-17','paid',8999),
  (4,2,'2026-03-21','cancelled',20000),(5,3,'2026-02-11','paid',3000),(6,3,'2026-03-30','pending',7500),
  (7,4,'2026-03-02','paid',15025),(8,4,'2026-04-18','paid',2000),(9,5,'2026-03-15','cancelled',9900),
  (10,5,'2026-05-06','paid',6400),(11,1,'2026-05-20','pending',1100),(12,2,'2026-06-01','paid',1001);
INSERT INTO refunds VALUES (3,2999,'2026-01-25'),(7,5025,'2026-03-10');
