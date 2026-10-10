import net.minecraft.SharedConstants;
import net.minecraft.core.Holder;
import net.minecraft.core.HolderLookup;
import net.minecraft.core.registries.Registries;
import net.minecraft.resources.Identifier;
import net.minecraft.server.Bootstrap;
import net.minecraft.world.level.biome.Biome;
import net.minecraft.world.level.biome.BiomeSource;
import net.minecraft.world.level.biome.Climate;
import net.minecraft.world.level.biome.MultiNoiseBiomeSource;
import net.minecraft.world.level.levelgen.NoiseGeneratorSettings;
import net.minecraft.world.level.levelgen.RandomState;
import net.minecraft.world.level.levelgen.densityfunction.DensityBufferPool;
import net.minecraft.world.level.levelgen.densityfunction.SamplerContext;

import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.List;

/**
 * ClimateOracle — the vanilla 26.3 ground truth for biome decisions, used to
 * verify LivingWorld's climate router evaluation and biome selection.
 *
 * For every (x, y, z) block coordinate listed in the input file it prints
 *   <x> <y> <z> <t> <h> <c> <e> <d> <w> <biomeId>
 * using RandomState.createClimateSampler + MultiNoiseBiomeSource's resolver —
 * the exact objects ChunkGenerator.doCreateBiomes builds — so a Go-side
 * comparison covers both the climate evaluation and the biome selection.
 *
 * Input: one "x y z" block coordinate per line (sampled at the surface quart,
 * i.e. the caller passes the y the surface biome should be sampled at).
 *
 * Usage: ClimateOracle <seed> <in.txt> <out.txt>
 */
public class ClimateOracle {
    public static void main(String[] args) throws Exception {
        if (args.length < 3) {
            System.out.println("usage: ClimateOracle <seed> <in.txt> <out.txt>");
            System.exit(2);
        }
        long seed = Long.parseLong(args[0]);
        SharedConstants.tryDetectVersion();
        Bootstrap.bootStrap();

        HolderLookup.Provider lookup = net.minecraft.data.registries.VanillaRegistries.createWorldLookup();

        Holder<NoiseGeneratorSettings> settingsHolder =
                lookup.lookupOrThrow(Registries.NOISE_SETTINGS)
                        .getOrThrow(net.minecraft.resources.ResourceKey.create(
                                Registries.NOISE_SETTINGS,
                                Identifier.parse("minecraft:overworld")));
        NoiseGeneratorSettings settings = settingsHolder.value();
        System.out.println("sea_level=" + settings.seaLevel()
                + " minY=" + settings.noiseSettings().minY()
                + " height=" + settings.noiseSettings().height());

        RandomState randomState = RandomState.create(
                lookup.lookupOrThrow(Registries.NOISE), seed, settings);
        DensityBufferPool pool = randomState.acquireDensityBufferPool();
        SamplerContext ctx = SamplerContext.builder()
                .enableCaches()
                .useBufferArena(pool)
                .build();
        Climate.Sampler sampler = randomState.createClimateSampler(ctx);
        BiomeSource biomeSource = MultiNoiseBiomeSource.createFromPreset(
                lookup.lookupOrThrow(Registries.MULTI_NOISE_BIOME_SOURCE_PARAMETER_LIST)
                        .getOrThrow(net.minecraft.resources.ResourceKey.create(
                                Registries.MULTI_NOISE_BIOME_SOURCE_PARAMETER_LIST,
                                Identifier.parse("minecraft:overworld"))));
        var resolver = biomeSource.createResolver(sampler);

        StringBuilder sb = new StringBuilder();
        for (String line : Files.readAllLines(Path.of(args[1]))) {
            String[] parts = line.trim().split("\\s+");
            if (parts.length < 3) continue;
            int x = Integer.parseInt(parts[0]);
            int y = Integer.parseInt(parts[1]);
            int z = Integer.parseInt(parts[2]);
            int qx = net.minecraft.core.QuartPos.fromBlock(x);
            int qy = net.minecraft.core.QuartPos.fromBlock(y);
            int qz = net.minecraft.core.QuartPos.fromBlock(z);
            Climate.TargetPoint t = sampler.sample(qx, qy, qz);
            Holder<Biome> biome = resolver.getNoiseBiome(qx, qy, qz);
            sb.append(x).append(' ').append(y).append(' ').append(z)
                    .append(' ').append(t.temperature())
                    .append(' ').append(t.humidity())
                    .append(' ').append(t.continentalness())
                    .append(' ').append(t.erosion())
                    .append(' ').append(t.depth())
                    .append(' ').append(t.weirdness())
                    .append(' ').append(biome.unwrapKey().map(k -> k.identifier().toString()).orElse("?"))
                    .append('\n');
        }
        Files.write(Path.of(args[2]), sb.toString().getBytes(StandardCharsets.UTF_8));
        randomState.releaseDensityBufferPool(pool);
        System.out.println("wrote oracle output for " + args[1]);
    }
}
