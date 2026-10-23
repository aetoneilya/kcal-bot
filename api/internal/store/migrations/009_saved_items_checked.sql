-- A saved meal was seen by its owner, so no item in it is "low" — unsure,
-- please check — any more. New saves do the same (store.insertItems).
UPDATE meal_items SET confidence = 'high' WHERE confidence = 'low';
