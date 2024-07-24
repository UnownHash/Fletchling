DROP PROCEDURE IF EXISTS fl_nest_filter_overlap;
CREATE PROCEDURE fl_nest_filter_overlap (IN maximum_overlap double)
BEGIN
  DROP TEMPORARY TABLE IF EXISTS overlapNest;
  IF (SELECT VERSION() LIKE '8.0%') THEN
    DROP TEMPORARY TABLE IF EXISTS convertedGeosA;
    DROP TEMPORARY TABLE IF EXISTS convertedGeosB;
    CREATE TEMPORARY TABLE convertedGeosA AS (
        SELECT nest_id, m2, ST_GeomFromText(ST_ASTEXT(polygon), 0) as polygon FROM nests WHERE active=1
    );
    CREATE TEMPORARY TABLE convertedGeosB AS (
        SELECT nest_id, m2, ST_GeomFromText(ST_ASTEXT(polygon), 0) as polygon FROM nests WHERE active=1
    );
    CREATE TEMPORARY TABLE overlapNest AS (
      SELECT b.nest_id
      FROM convertedGeosA a, convertedGeosB b
      WHERE a.m2 > b.m2 AND
        ST_Intersects(a.polygon, b.polygon) AND
        ST_GeometryType(ST_Intersection(a.polygon, b.polygon)) IN ('Polygon', 'MultiPolygon') AND
        (100 * ST_Area(ST_Intersection(a.polygon,b.polygon)) / ST_Area(b.polygon)) > maximum_overlap
    );
  ELSE
    CREATE TEMPORARY TABLE overlapNest AS (
      SELECT b.nest_id
      FROM nests a, nests b
      WHERE a.active = 1 AND b.active = 1 AND
        a.m2 > b.m2 AND
        ST_Intersects(a.polygon, b.polygon) AND
        ST_GeometryType(ST_Intersection(a.polygon, b.polygon)) IN ('Polygon', 'MultiPolygon') AND
        (100 * ST_Area(ST_Intersection(a.polygon,b.polygon)) / ST_Area(b.polygon)) > maximum_overlap
    );
  END IF;
  UPDATE nests a, overlapNest b SET a.active=0,a.discarded='overlap',a.pokemon_id=NULL,a.pokemon_form=NULL,a.pokemon_avg=NULL,a.pokemon_count=NULL,a.pokemon_ratio=NULL,a.pokemon_avg=NULL WHERE a.nest_id=b.nest_id;
  DROP TEMPORARY TABLE overlapNest;
  DROP TEMPORARY TABLE IF EXISTS convertedGeosA;
  DROP TEMPORARY TABLE IF EXISTS convertedGeosB;
END
