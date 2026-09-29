import net.minecraft.world.level.levelgen.RandomSupport;

/** Prints RandomSupport.seedFromHashOf components so the Go port can be checked. */
public class SeedProbe {
    public static void main(String[] args) {
        for (String s : args.length > 0 ? args : new String[]{
                "minecraft:continentalness", "octave_-9", "octave_0"}) {
            var seed = RandomSupport.seedFromHashOf(s);
            System.out.println(s + " lo=" + seed.seedLo() + " hi=" + seed.seedHi());
        }
    }
}
